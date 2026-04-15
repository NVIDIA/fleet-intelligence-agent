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

package attestationloop

import (
	"context"
	"fmt"
	"time"
)

// JWTProvider retrieves the current backend JWT.
type JWTProvider interface {
	GetJWT(ctx context.Context) (string, error)
	SetJWT(ctx context.Context, value string) error
}

// Manager coordinates periodic attestation collection into a store.
type Manager interface {
	Run(ctx context.Context) error
	CollectOnce(ctx context.Context) (*Result, error)
}

type manager struct {
	nodeIDProvider func(context.Context) (string, error)
	jwtProvider    JWTProvider
	nonceProvider  NonceProvider
	collector      EvidenceCollector
	store          StateStore
	interval       time.Duration
}

// NewManager creates an attestation loop manager skeleton.
func NewManager(
	nodeIDProvider func(context.Context) (string, error),
	jwtProvider JWTProvider,
	nonceProvider NonceProvider,
	collector EvidenceCollector,
	store StateStore,
	interval time.Duration,
) Manager {
	return &manager{
		nodeIDProvider: nodeIDProvider,
		jwtProvider:    jwtProvider,
		nonceProvider:  nonceProvider,
		collector:      collector,
		store:          store,
		interval:       interval,
	}
}

func (m *manager) Run(ctx context.Context) error {
	if _, err := m.CollectOnce(ctx); err != nil {
		return err
	}
	return fmt.Errorf("attestation loop run loop not implemented")
}

func (m *manager) CollectOnce(ctx context.Context) (*Result, error) {
	if m.nodeIDProvider == nil || m.jwtProvider == nil || m.nonceProvider == nil || m.collector == nil {
		return nil, fmt.Errorf("attestation loop dependencies are incomplete")
	}

	nodeID, err := m.nodeIDProvider(ctx)
	if err != nil {
		return nil, err
	}
	jwt, err := m.jwtProvider.GetJWT(ctx)
	if err != nil {
		return nil, err
	}
	nonce, refreshTS, refreshedJWT, err := m.nonceProvider.GetNonce(ctx, nodeID, jwt)
	if err != nil {
		return nil, err
	}
	if refreshedJWT != "" && refreshedJWT != jwt {
		if err := m.jwtProvider.SetJWT(ctx, refreshedJWT); err != nil {
			return nil, err
		}
	}
	sdkResp, err := m.collector.Collect(ctx, nonce)
	if err != nil {
		return nil, err
	}
	result := &Result{
		CollectedAt:           time.Now().UTC(),
		NodeID:                nodeID,
		NonceRefreshTimestamp: refreshTS,
		Success:               true,
	}
	if sdkResp != nil {
		result.SDKResponse = *sdkResp
	}
	if m.store != nil {
		if err := m.store.PutAttestation(ctx, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}
