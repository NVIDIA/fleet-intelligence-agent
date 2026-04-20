// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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

// Package sink contains inventory sink implementations.
package sink

import (
	"context"
	"fmt"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/agentstate"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/backendclient"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/inventory"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/inventory/mapper"
)

type backendSink struct {
	state         agentstate.State
	clientFactory func(rawBaseURL string) (backendclient.Client, error)
}

// NewBackendSink creates the backend inventory sink.
func NewBackendSink(state agentstate.State) inventory.Sink {
	return &backendSink{
		state:         state,
		clientFactory: backendclient.New,
	}
}

func (s *backendSink) Export(ctx context.Context, snap *inventory.Snapshot) error {
	if s.state == nil {
		return fmt.Errorf("inventory backend export requires agent state")
	}
	if s.clientFactory == nil {
		return fmt.Errorf("inventory backend export requires backend client factory")
	}
	if snap == nil {
		return fmt.Errorf("inventory backend export requires inventory snapshot")
	}
	baseURL, ok, err := s.state.GetBackendBaseURL(ctx)
	if err != nil {
		return err
	}
	if !ok || baseURL == "" {
		return inventory.ErrNotReady
	}
	jwt, ok, err := s.state.GetJWT(ctx)
	if err != nil {
		return err
	}
	if !ok || jwt == "" {
		return inventory.ErrNotReady
	}
	nodeUUID, ok, err := s.state.GetNodeID(ctx)
	if err != nil {
		return err
	}
	if !ok || nodeUUID == "" {
		return inventory.ErrNotReady
	}
	client, err := s.clientFactory(baseURL)
	if err != nil {
		return fmt.Errorf("create backend client: %w", err)
	}
	return client.UpsertNode(ctx, nodeUUID, mapper.ToNodeUpsertRequest(snap), jwt)
}
