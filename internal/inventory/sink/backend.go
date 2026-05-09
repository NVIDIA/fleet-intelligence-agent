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

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"

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
		log.Logger.Infow("inventory export skipped: agent not enrolled (no backend URL)")
		return inventory.ErrNotReady
	}
	jwt, ok, err := s.state.GetJWT(ctx)
	if err != nil {
		return err
	}
	if !ok || jwt == "" {
		log.Logger.Infow("inventory export skipped: agent not enrolled (no JWT)")
		return inventory.ErrNotReady
	}
	nodeUUID, ok, err := s.state.GetNodeUUID(ctx)
	if err != nil {
		return err
	}
	if !ok || nodeUUID == "" {
		log.Logger.Infow("inventory export skipped: agent not enrolled (no node UUID)")
		return inventory.ErrNotReady
	}
	client, err := s.clientFactory(baseURL)
	if err != nil {
		return fmt.Errorf("create backend client: %w", err)
	}
	req := mapper.ToNodeUpsertRequest(snap)
	enrollmentTime, ok, err := s.state.GetEnrollmentTime(ctx)
	if err != nil {
		log.Logger.Warnw("inventory export continuing without enrollment time", "error", err)
	} else if ok && !enrollmentTime.IsZero() {
		normalized := enrollmentTime.UTC()
		req.EnrolledAt = &normalized
	}
	if err := client.UpsertNode(ctx, nodeUUID, req, jwt); err != nil {
		return err
	}
	log.Logger.Infow("inventory exported to backend", "node_uuid", nodeUUID)
	return nil
}
