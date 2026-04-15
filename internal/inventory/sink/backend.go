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

	"github.com/NVIDIA/fleet-intelligence-agent/internal/backendclient"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/inventory"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/inventory/mapper"
)

type backendSink struct {
	client backendclient.Client
	jwt    func(context.Context) (string, error)
}

// NewBackendSink creates the backend inventory sink skeleton.
func NewBackendSink(client backendclient.Client, jwt func(context.Context) (string, error)) inventory.Sink {
	return &backendSink{
		client: client,
		jwt:    jwt,
	}
}

func (s *backendSink) Export(ctx context.Context, snap inventory.Snapshot) error {
	jwt, err := s.jwt(ctx)
	if err != nil {
		return err
	}
	return s.client.UpsertNode(ctx, snap.NodeID, mapper.ToNodeUpsertRequest(snap), jwt)
}
