// SPDX-FileCopyrightText: Copyright (c) 2025, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package enrollment provides shared enrollment functionality for the Fleet Intelligence agent.
package enrollment

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	pkgmetadata "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metadata"
	nvidianvml "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/nvml"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/sqlite"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/agentstate"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/backendclient"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/config"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/endpoint"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/inventory"
	inventorysink "github.com/NVIDIA/fleet-intelligence-agent/internal/inventory/sink"
	inventorysource "github.com/NVIDIA/fleet-intelligence-agent/internal/inventory/source"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/machineinfo"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/registry"
	agenttag "github.com/NVIDIA/fleet-intelligence-agent/internal/tag"
)

var (
	newBackendClient               = backendclient.New
	syncInventoryAfterEnroll       = syncInventoryOnce
	syncTagsAfterEnroll            = syncTagsOnce
	postEnrollInventorySyncTimeout = time.Minute
)

// Enroll runs the full enrollment workflow and performs a best-effort initial inventory sync.
func Enroll(ctx context.Context, baseEndpoint, sakToken string) error {
	return EnrollWithConfig(ctx, baseEndpoint, sakToken, nil)
}

// EnrollWithConfig runs the full enrollment workflow and uses cfg for best-effort inventory metadata.
func EnrollWithConfig(ctx context.Context, baseEndpoint, sakToken string, cfg *config.Config) error {
	configuredTags, err := agenttag.ParseFromEnv()
	if err != nil {
		return fmt.Errorf("invalid agent tag configuration: %w", err)
	}

	baseURL, err := normalizeBackendBaseURL(baseEndpoint)
	if err != nil {
		return fmt.Errorf("invalid enrollment endpoint: %w", err)
	}

	client, err := newBackendClient(baseURL.String())
	if err != nil {
		return fmt.Errorf("failed to create backend client: %w", err)
	}
	jwtToken, err := client.Enroll(ctx, sakToken)
	if err != nil {
		return err
	}
	enrolledAt := time.Now().UTC()
	if err := storeConfigInMetadata(ctx, baseURL.String(), jwtToken, sakToken, enrolledAt); err != nil {
		return fmt.Errorf("failed to store configuration: %w", err)
	}
	tagSeedPatch, err := seedTagsInMetadata(ctx, agentstate.NewSQLite(), configuredTags)
	if err != nil {
		return fmt.Errorf("failed to seed tags metadata: %w", err)
	}
	inventorySyncCtx, inventorySyncCancel := context.WithTimeout(ctx, postEnrollInventorySyncTimeout)
	defer inventorySyncCancel()
	if err := runWithContext(inventorySyncCtx, func() error {
		return syncInventoryAfterEnroll(inventorySyncCtx, cfg)
	}); err != nil {
		log.Logger.Warnw("post-enroll inventory sync failed", "error", err)
	}
	if len(tagSeedPatch) > 0 {
		tagSyncCtx, tagSyncCancel := context.WithTimeout(ctx, postEnrollInventorySyncTimeout)
		defer tagSyncCancel()
		if err := runWithContext(tagSyncCtx, func() error {
			return syncTagsAfterEnroll(tagSyncCtx, tagSeedPatch)
		}); err != nil {
			log.Logger.Warnw("post-enroll tag sync failed", "error", err)
		}
	}
	return nil
}

func runWithContext(ctx context.Context, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func normalizeBackendBaseURL(raw string) (*url.URL, error) {
	baseURL, err := endpoint.ValidateBackendEndpoint(raw)
	if err != nil {
		return nil, err
	}
	if baseURL.Path == "" || baseURL.Path == "/" {
		return baseURL, nil
	}

	normalized, err := endpoint.DeriveBackendBaseURL(raw)
	if err != nil {
		return nil, err
	}
	return endpoint.ValidateBackendEndpoint(normalized)
}

func storeConfigInMetadata(
	ctx context.Context,
	baseURL,
	jwtToken,
	sakToken string,
	enrolledAt time.Time,
) error {
	stateFile, err := config.DefaultStateFile()
	if err != nil {
		return fmt.Errorf("failed to get state file path: %w", err)
	}

	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state database: %w", err)
	}
	defer dbRW.Close()

	if err := config.SecureStateFilePermissions(stateFile); err != nil {
		return fmt.Errorf("failed to secure state database permissions: %w", err)
	}
	if err := pkgmetadata.CreateTableMetadata(ctx, dbRW); err != nil {
		return fmt.Errorf("failed to create metadata table: %w", err)
	}

	if err := pkgmetadata.SetMetadata(ctx, dbRW, agentstate.MetadataKeySAKToken, sakToken); err != nil {
		return fmt.Errorf("failed to set SAK token: %w", err)
	}
	if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyToken, jwtToken); err != nil {
		return fmt.Errorf("failed to set JWT token: %w", err)
	}
	if err := pkgmetadata.SetMetadata(ctx, dbRW, agentstate.MetadataKeyBackendBaseURL, baseURL); err != nil {
		return fmt.Errorf("failed to set backend base URL: %w", err)
	}
	if err := pkgmetadata.SetMetadata(ctx, dbRW, agentstate.MetadataKeyEnrolledAt, enrolledAt.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("failed to set enrollment time: %w", err)
	}
	return nil
}

func seedTagsInMetadata(ctx context.Context, state agentstate.State, configuredTags map[string]string) (map[string]string, error) {
	if state == nil {
		return nil, fmt.Errorf("agent state is required for tag seeding")
	}

	existingTags, ok, err := state.GetTags(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		existingTags = map[string]string{}
	}
	seeded := agenttag.Clone(existingTags)
	seedPatch := map[string]string{}
	for key, value := range configuredTags {
		if _, exists := seeded[key]; exists {
			continue
		}
		seeded[key] = value
		seedPatch[key] = value
	}
	if ok && stringMapEqual(existingTags, seeded) {
		return seedPatch, nil
	}
	if err := state.SetTags(ctx, seeded); err != nil {
		return nil, err
	}
	return seedPatch, nil
}

type machineInfoCollectorFunc func(context.Context) (*machineinfo.MachineInfo, error)

func (f machineInfoCollectorFunc) Collect(ctx context.Context) (*machineinfo.MachineInfo, error) {
	return f(ctx)
}

// UpsertTagsNow sends a patch of tag updates to backend.
func UpsertTagsNow(ctx context.Context, updates map[string]string) error {
	return syncTagsOnce(ctx, updates)
}

func syncInventoryOnce(ctx context.Context, cfg *config.Config) error {
	state := agentstate.NewSQLite()
	sink := inventorysink.NewBackendSink(state)
	allComponents := registry.AllComponentNames()

	if cfg == nil {
		defaultCfg, err := config.Default(ctx)
		if err != nil {
			return fmt.Errorf("load default config for inventory sync: %w", err)
		}
		cfg = defaultCfg
	}
	retentionPeriodSeconds, enabledComponents, disabledComponents := cfg.InventoryAgentConfig(allComponents)
	inventoryEnabled, inventoryIntervalSeconds := cfg.InventoryLoopAgentConfig()
	attestationEnabled, attestationIntervalSeconds := cfg.AttestationLoopAgentConfig()

	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		return fmt.Errorf("initialize nvml for inventory sync: %w", err)
	}
	defer func() { _ = nvmlInstance.Shutdown() }()

	src := inventorysource.NewMachineInfoSourceWithAgentConfig(
		machineInfoCollectorFunc(func(context.Context) (*machineinfo.MachineInfo, error) {
			return machineinfo.GetMachineInfo(nvmlInstance)
		}),
		&inventory.AgentConfig{
			TotalComponents:            int64(len(allComponents)),
			RetentionPeriodSeconds:     retentionPeriodSeconds,
			EnabledComponents:          enabledComponents,
			DisabledComponents:         disabledComponents,
			InventoryEnabled:           inventoryEnabled,
			InventoryIntervalSeconds:   inventoryIntervalSeconds,
			AttestationEnabled:         attestationEnabled,
			AttestationIntervalSeconds: attestationIntervalSeconds,
		},
	)
	manager := inventory.NewManager(src, sink, inventory.InventoryConfig{})
	_, err = manager.CollectOnce(ctx)
	return err
}

func syncTagsOnce(ctx context.Context, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	if err := agenttag.ValidateReservedPairPatch(updates); err != nil {
		return fmt.Errorf("invalid reserved tag combination: %w", err)
	}
	state := agentstate.NewSQLite()
	baseURL, ok, err := state.GetBackendBaseURL(ctx)
	if err != nil {
		return fmt.Errorf("read backend base URL from state: %w", err)
	}
	if !ok || baseURL == "" {
		return fmt.Errorf("agent not enrolled: backend base URL is missing")
	}
	jwt, ok, err := state.GetJWT(ctx)
	if err != nil {
		return fmt.Errorf("read JWT from state: %w", err)
	}
	if !ok || jwt == "" {
		return fmt.Errorf("agent not enrolled: JWT is missing")
	}
	nodeUUID, ok, err := state.GetNodeUUID(ctx)
	if err != nil {
		return fmt.Errorf("read node UUID from state: %w", err)
	}
	if !ok || nodeUUID == "" {
		return fmt.Errorf("agent not enrolled: node UUID is missing")
	}

	client, err := newBackendClient(baseURL)
	if err != nil {
		return fmt.Errorf("create backend client: %w", err)
	}
	req := toNodeTagsUpsertRequest(updates)
	keys := tagKeys(updates)
	log.Logger.Infow("sending node tags upsert request",
		"request", req,
	)
	if err := client.UpsertNodeTags(ctx, nodeUUID, req, jwt); err != nil {
		return fmt.Errorf("upsert node tags: %w", err)
	}
	log.Logger.Infow("tags exported to backend",
		"node_uuid", nodeUUID,
		"tag_count", len(keys),
		"tag_keys", keys,
	)
	return nil
}

func stringMapEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		if rightValue, ok := right[key]; !ok || rightValue != leftValue {
			return false
		}
	}
	return true
}

func tagKeys(tags map[string]string) []string {
	if len(tags) == 0 {
		return nil
	}
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func toNodeTagsUpsertRequest(updates map[string]string) *backendclient.NodeTagsUpsertRequest {
	req := &backendclient.NodeTagsUpsertRequest{}
	if value, ok := updates[agenttag.ReservedKeyNodeGroup]; ok {
		v := value
		req.NodeGroup = &v
	}
	if value, ok := updates[agenttag.ReservedKeyComputeZone]; ok {
		v := value
		req.ComputeZone = &v
	}
	for key, value := range updates {
		if key == agenttag.ReservedKeyNodeGroup || key == agenttag.ReservedKeyComputeZone {
			continue
		}
		if strings.TrimSpace(value) == "" {
			req.CustomRemove = append(req.CustomRemove, key)
			continue
		}
		if req.CustomSet == nil {
			req.CustomSet = map[string]string{}
		}
		req.CustomSet[key] = value
	}
	if len(req.CustomRemove) > 0 {
		sort.Strings(req.CustomRemove)
	}
	return req
}
