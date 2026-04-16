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
	"sync"
	"time"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/agentstate"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/backendclient"
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
	LastResult() *Result
	IsResultUpdated(since time.Time) bool
}

type manager struct {
	mu             sync.RWMutex
	nodeIDProvider func(context.Context) (string, error)
	jwtProvider    JWTProvider
	nonceProvider  NonceProvider
	collector      EvidenceCollector
	submitter      Submitter
	interval       time.Duration

	lastResult  *Result
	lastUpdated time.Time
}

// NewManager creates an attestation loop manager skeleton.
func NewManager(
	nodeIDProvider func(context.Context) (string, error),
	jwtProvider JWTProvider,
	nonceProvider NonceProvider,
	collector EvidenceCollector,
	submitter Submitter,
	interval time.Duration,
) Manager {
	return &manager{
		nodeIDProvider: nodeIDProvider,
		jwtProvider:    jwtProvider,
		nonceProvider:  nonceProvider,
		collector:      collector,
		submitter:      submitter,
		interval:       interval,
	}
}

func (m *manager) Run(ctx context.Context) error {
	if m.nodeIDProvider == nil || m.jwtProvider == nil || m.nonceProvider == nil || m.collector == nil || m.submitter == nil {
		return fmt.Errorf("attestation loop dependencies are incomplete")
	}
	if _, err := m.CollectOnce(ctx); err != nil {
		log.Logger.Warnw("initial attestation workflow failed", "error", err)
	}
	if m.interval <= 0 {
		return nil
	}

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := m.CollectOnce(ctx); err != nil {
				log.Logger.Warnw("periodic attestation workflow failed", "error", err)
			}
		}
	}
}

func (m *manager) CollectOnce(ctx context.Context) (*Result, error) {
	if m.nodeIDProvider == nil || m.jwtProvider == nil || m.nonceProvider == nil || m.collector == nil || m.submitter == nil {
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
		jwt = refreshedJWT
	}
	sdkResp, err := m.collector.Collect(ctx, nonce)
	result := &Result{
		CollectedAt:           time.Now().UTC(),
		NodeID:                nodeID,
		NonceRefreshTimestamp: refreshTS,
	}
	if err != nil {
		result.Success = false
		result.ErrorMessage = err.Error()
	} else {
		result.Success = true
	}
	if sdkResp != nil {
		result.SDKResponse = *sdkResp
	}
	m.mu.Lock()
	cloned := *result
	m.lastResult = &cloned
	m.lastUpdated = time.Now().UTC()
	m.mu.Unlock()
	if err := m.submitter.Submit(ctx, result, jwt); err != nil {
		return nil, err
	}
	return result, nil
}

func (m *manager) LastResult() *Result {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.lastResult == nil {
		return nil
	}
	cloned := *m.lastResult
	return &cloned
}

func (m *manager) IsResultUpdated(since time.Time) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastUpdated.After(since)
}

type backendSubmitter struct {
	client BackendClient
}

// BackendClient is the backend client view required by the attestation workflow.
type BackendClient interface {
	SubmitAttestation(ctx context.Context, nodeID string, req *backendclient.AttestationRequest, jwt string) error
}

// NewBackendSubmitter creates a backend submitter backed by the agent backend client.
func NewBackendSubmitter(client BackendClient) Submitter {
	return &backendSubmitter{client: client}
}

func (s *backendSubmitter) Submit(ctx context.Context, result *Result, jwt string) error {
	if s.client == nil {
		return fmt.Errorf("attestation submission requires backend client")
	}
	if result == nil {
		return fmt.Errorf("attestation submission requires result")
	}
	if jwt == "" {
		return fmt.Errorf("attestation submission requires jwt")
	}
	return s.client.SubmitAttestation(ctx, result.NodeID, toAttestationRequest(result), jwt)
}

type stateJWTProvider struct {
	state agentstate.State
}

// NewStateJWTProvider returns a JWT provider backed by persisted agent state.
func NewStateJWTProvider(state agentstate.State) JWTProvider {
	return &stateJWTProvider{state: state}
}

func (p *stateJWTProvider) GetJWT(ctx context.Context) (string, error) {
	if p.state == nil {
		return "", fmt.Errorf("jwt provider requires agent state")
	}
	value, ok, err := p.state.GetJWT(ctx)
	if err != nil {
		return "", err
	}
	if !ok || value == "" {
		return "", fmt.Errorf("jwt not available in agent state")
	}
	return value, nil
}

func (p *stateJWTProvider) SetJWT(ctx context.Context, value string) error {
	if p.state == nil {
		return fmt.Errorf("jwt provider requires agent state")
	}
	return p.state.SetJWT(ctx, value)
}

// NewStateNodeIDProvider returns a node ID provider backed by persisted agent state.
func NewStateNodeIDProvider(state agentstate.State) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		if state == nil {
			return "", fmt.Errorf("node ID provider requires agent state")
		}
		value, ok, err := state.GetNodeID(ctx)
		if err != nil {
			return "", err
		}
		if !ok || value == "" {
			return "", fmt.Errorf("node ID not available in agent state")
		}
		return value, nil
	}
}
