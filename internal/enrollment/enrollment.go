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
	"time"

	pkghost "github.com/NVIDIA/fleet-intelligence-sdk/pkg/host"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	pkgmetadata "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metadata"
	nvidianvml "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/nvml"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/sqlite"
	"github.com/google/uuid"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/agentstate"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/backendclient"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/config"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/endpoint"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/inventory"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/inventory/mapper"
	inventorysource "github.com/NVIDIA/fleet-intelligence-agent/internal/inventory/source"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/machineinfo"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/registry"
)

var (
	newBackendClient               = backendclient.New
	syncInventoryAfterEnroll       = syncInventoryOnce
	storeEnrollmentConfig          = storeConfigInMetadata
	postEnrollInventorySyncTimeout = time.Minute
)

// Enroll runs the full enrollment workflow and performs a best-effort initial inventory sync.
func Enroll(ctx context.Context, baseEndpoint, sakToken string) error {
	return EnrollWithConfig(ctx, baseEndpoint, sakToken, nil)
}

// EnrollWithConfig runs the full enrollment workflow and uses cfg for best-effort inventory metadata.
func EnrollWithConfig(ctx context.Context, baseEndpoint, sakToken string, cfg *config.Config) error {
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

	nodeUUID := resolveNodeUUID(ctx)
	if nodeUUID == "" {
		return fmt.Errorf("failed to resolve node UUID")
	}

	syncCtx, cancel := context.WithTimeout(ctx, postEnrollInventorySyncTimeout)
	defer cancel()
	if err := runWithContext(syncCtx, func() error {
		return syncInventoryAfterEnroll(syncCtx, client, nodeUUID, jwtToken, cfg)
	}); err != nil {
		return fmt.Errorf("initial node upsert failed: %w", err)
	}

	if err := storeEnrollmentConfig(ctx, baseURL.String(), jwtToken, sakToken, nodeUUID); err != nil {
		return fmt.Errorf("failed to store configuration: %w", err)
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

func storeConfigInMetadata(ctx context.Context, baseURL, jwtToken, sakToken, nodeUUID string) error {
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
	if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyMachineID, nodeUUID); err != nil {
		return fmt.Errorf("failed to set node UUID: %w", err)
	}
	return nil
}

func resolveNodeUUID(ctx context.Context) string {
	state := agentstate.NewSQLite()
	if existing, ok, err := state.GetNodeUUID(ctx); err == nil && ok && existing != "" {
		if _, parseErr := uuid.Parse(existing); parseErr == nil {
			return existing
		}
		log.Logger.Warnw("ignoring invalid persisted node UUID", "node_uuid", existing)
	} else if err != nil {
		log.Logger.Warnw("failed to read persisted node UUID; generating a new one", "error", err)
	}

	nodeUUID, err := pkghost.GetDmidecodeUUID(ctx)
	if err == nil && nodeUUID != "" {
		if _, parseErr := uuid.Parse(nodeUUID); parseErr == nil {
			return nodeUUID
		}
		log.Logger.Warnw("ignoring invalid hardware node UUID", "node_uuid", nodeUUID)
	} else if err != nil {
		log.Logger.Warnw("failed to get hardware node UUID; generating a new one", "error", err)
	}

	return uuid.NewString()
}

type machineInfoCollectorFunc func(context.Context) (*machineinfo.MachineInfo, error)

func (f machineInfoCollectorFunc) Collect(ctx context.Context) (*machineinfo.MachineInfo, error) {
	return f(ctx)
}

type initialNodeUpsertSink struct {
	client   backendclient.Client
	nodeUUID string
	jwt      string
}

func (s *initialNodeUpsertSink) Export(ctx context.Context, snap *inventory.Snapshot) error {
	if s.client == nil {
		return fmt.Errorf("initial node upsert requires backend client")
	}
	if s.nodeUUID == "" {
		return fmt.Errorf("initial node upsert requires node UUID")
	}
	if s.jwt == "" {
		return fmt.Errorf("initial node upsert requires JWT")
	}
	return s.client.UpsertNode(ctx, s.nodeUUID, mapper.ToNodeUpsertRequest(snap), s.jwt)
}

func buildInventorySource(ctx context.Context, cfg *config.Config) (inventory.Source, func(), error) {
	allComponents := registry.AllComponentNames()

	if cfg == nil {
		var err error
		cfg, err = config.Default(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("load default config for inventory sync: %w", err)
		}
	}
	retentionPeriodSeconds, enabledComponents, disabledComponents := cfg.InventoryAgentConfig(allComponents)
	inventoryEnabled, inventoryIntervalSeconds := cfg.InventoryLoopAgentConfig()
	attestationEnabled, attestationIntervalSeconds := cfg.AttestationLoopAgentConfig()

	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		return nil, nil, fmt.Errorf("initialize nvml for inventory sync: %w", err)
	}

	return inventorysource.NewMachineInfoSourceWithAgentConfig(
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
	), func() { _ = nvmlInstance.Shutdown() }, nil
}

func syncInventoryOnce(ctx context.Context, client backendclient.Client, nodeUUID, jwt string, cfg *config.Config) error {
	src, cleanup, err := buildInventorySource(ctx, cfg)
	if err != nil {
		return err
	}
	defer cleanup()
	sink := &initialNodeUpsertSink{
		client:   client,
		nodeUUID: nodeUUID,
		jwt:      jwt,
	}
	manager := inventory.NewManager(src, sink, inventory.InventoryConfig{})
	_, err = manager.CollectOnce(ctx)
	return err
}
