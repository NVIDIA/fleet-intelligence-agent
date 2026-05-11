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
)

var (
	newBackendClient               = backendclient.New
	syncInventoryAfterEnroll       = syncInventoryOnce
	postEnrollInventorySyncTimeout = time.Minute
)

// WorkflowOutcome captures non-fatal post-enrollment workflow results.
type WorkflowOutcome struct {
	// InitialInventorySyncError is set when the best-effort initial inventory sync fails.
	// Enrollment itself still succeeds in this case.
	InitialInventorySyncError error
}

// Enroll runs the full enrollment workflow and performs a best-effort initial inventory sync.
func Enroll(ctx context.Context, baseEndpoint, sakToken string) error {
	_, err := EnrollWithConfig(ctx, baseEndpoint, sakToken, nil)
	return err
}

// EnrollWithConfig runs enrollment and returns non-fatal post-enroll outcomes.
func EnrollWithConfig(ctx context.Context, baseEndpoint, sakToken string, cfg *config.Config) (WorkflowOutcome, error) {
	outcome := WorkflowOutcome{}

	baseURL, err := normalizeBackendBaseURL(baseEndpoint)
	if err != nil {
		return outcome, fmt.Errorf("invalid enrollment endpoint: %w", err)
	}

	client, err := newBackendClient(baseURL.String())
	if err != nil {
		return outcome, fmt.Errorf("failed to create backend client: %w", err)
	}
	jwtToken, err := client.Enroll(ctx, sakToken)
	if err != nil {
		return outcome, err
	}
	enrolledAt := time.Now().UTC()
	if err := storeConfigInMetadata(ctx, baseURL.String(), jwtToken, sakToken, enrolledAt); err != nil {
		return outcome, fmt.Errorf("failed to store configuration: %w", err)
	}
	syncCtx, cancel := context.WithTimeout(ctx, postEnrollInventorySyncTimeout)
	defer cancel()
	if err := runWithContext(syncCtx, func() error {
		return syncInventoryAfterEnroll(syncCtx, cfg)
	}); err != nil {
		outcome.InitialInventorySyncError = err
		log.Logger.Warnw("post-enroll inventory sync failed", "error", err)
	}
	return outcome, nil
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

func storeConfigInMetadata(ctx context.Context, baseURL, jwtToken, sakToken string, enrolledAt time.Time) error {
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

type machineInfoCollectorFunc func(context.Context) (*machineinfo.MachineInfo, error)

func (f machineInfoCollectorFunc) Collect(ctx context.Context) (*machineinfo.MachineInfo, error) {
	return f(ctx)
}

func syncInventoryOnce(ctx context.Context, cfg *config.Config) error {
	state := agentstate.NewSQLite()
	sink := inventorysink.NewBackendSink(state)
	allComponents := registry.AllComponentNames()

	if cfg == nil {
		var err error
		cfg, err = config.Default(ctx)
		if err != nil {
			return fmt.Errorf("load default config for inventory sync: %w", err)
		}
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
