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
)

var (
	newBackendClient         = backendclient.New
	syncInventoryAfterEnroll = syncInventoryOnce
)

// Enroll runs the full enrollment workflow and performs a best-effort initial inventory sync.
func Enroll(ctx context.Context, baseEndpoint, sakToken string) error {
	baseURL, err := endpoint.ValidateBackendEndpoint(baseEndpoint)
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
	if err := storeConfigInMetadata(ctx, baseURL.String(), jwtToken, sakToken); err != nil {
		return fmt.Errorf("failed to store configuration: %w", err)
	}
	if err := syncInventoryAfterEnroll(ctx); err != nil {
		log.Logger.Warnw("post-enroll inventory sync failed", "error", err)
	}
	return nil
}

func storeConfigInMetadata(ctx context.Context, baseURL, jwtToken, sakToken string) error {
	stateFile, err := config.DefaultStateFile()
	if err != nil {
		return fmt.Errorf("failed to get state file path: %w", err)
	}

	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state database: %w", err)
	}
	defer dbRW.Close()

	if err := pkgmetadata.CreateTableMetadata(ctx, dbRW); err != nil {
		return fmt.Errorf("failed to create metadata table: %w", err)
	}

	state := agentstate.NewSQLite()
	if err := state.SetSAK(ctx, sakToken); err != nil {
		return fmt.Errorf("failed to set SAK token: %w", err)
	}
	if err := state.SetJWT(ctx, jwtToken); err != nil {
		return fmt.Errorf("failed to set JWT token: %w", err)
	}
	if err := state.SetBackendBaseURL(ctx, baseURL); err != nil {
		return fmt.Errorf("failed to set backend base URL: %w", err)
	}
	if err := config.SecureStateFilePermissions(stateFile); err != nil {
		return fmt.Errorf("failed to secure state database permissions: %w", err)
	}
	return nil
}

type machineInfoCollectorFunc func(context.Context) (*machineinfo.MachineInfo, error)

func (f machineInfoCollectorFunc) Collect(ctx context.Context) (*machineinfo.MachineInfo, error) {
	return f(ctx)
}

func syncInventoryOnce(ctx context.Context) error {
	state := agentstate.NewSQLite()
	sink := inventorysink.NewBackendSink(state)

	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		return fmt.Errorf("initialize nvml for inventory sync: %w", err)
	}
	defer func() { _ = nvmlInstance.Shutdown() }()

	src := inventorysource.NewMachineInfoSource(machineInfoCollectorFunc(func(context.Context) (*machineinfo.MachineInfo, error) {
		return machineinfo.GetMachineInfo(nvmlInstance)
	}))
	manager := inventory.NewManager(src, sink, 0)
	_, err = manager.CollectOnce(ctx)
	return err
}
