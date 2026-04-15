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

// Package attestationloop owns attestation collection and sync state orchestration.
package attestationloop

import (
	"context"
	"time"
)

// Result is the agent-owned attestation state model for the new backend sync loop.
type Result struct {
	CollectedAt           time.Time
	NodeID                string
	NonceRefreshTimestamp time.Time
	Success               bool
	ErrorMessage          string
	SDKResponse           SDKResponse
}

type SDKResponse struct {
	Evidences     []EvidenceItem
	ResultCode    int
	ResultMessage string
}

type EvidenceItem struct {
	Arch          string
	Certificate   string
	DriverVersion string
	Evidence      string
	Nonce         string
	VBIOSVersion  string
	Version       string
}

// NonceProvider retrieves a backend nonce for a node.
type NonceProvider interface {
	GetNonce(ctx context.Context, nodeID, jwt string) (nonce string, refreshTS time.Time, refreshedJWT string, err error)
}

// EvidenceCollector collects attestation evidence from local tooling.
type EvidenceCollector interface {
	Collect(ctx context.Context, nonce string) (*SDKResponse, error)
}

// Sink exports attestation results to an external destination.
type Sink interface {
	Export(ctx context.Context, result *Result) error
}

// StateStore is the attestation loop view of local transient store state.
type StateStore interface {
	PutAttestation(ctx context.Context, result *Result) error
	GetAttestation(ctx context.Context) (*Result, bool, error)
	MarkAttestationExported(ctx context.Context, key string, at time.Time) error
	WasAttestationExported(ctx context.Context, key string) (bool, error)
}
