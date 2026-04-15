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

// Package store contains transient in-agent state stores.
package store

import (
	"context"
	"sync"
	"time"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/attestationloop"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/inventory"
)

// MemoryStore is the initial in-memory implementation for inventory and attestation state.
type MemoryStore struct {
	mu sync.RWMutex

	inventory           *inventory.Snapshot
	hasInventory        bool
	lastInventoryHash   string
	lastInventorySyncTS time.Time

	attestation             *attestationloop.Result
	hasAttestation          bool
	exportedAttestationKeys map[string]time.Time
}

// NewMemoryStore creates an empty in-memory state store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		exportedAttestationKeys: make(map[string]time.Time),
	}
}

func (s *MemoryStore) PutInventory(_ context.Context, snap *inventory.Snapshot) error {
	if snap == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := *snap
	s.inventory = &cloned
	s.hasInventory = true
	return nil
}

func (s *MemoryStore) GetInventory(_ context.Context) (*inventory.Snapshot, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.hasInventory || s.inventory == nil {
		return nil, false, nil
	}
	cloned := *s.inventory
	return &cloned, true, nil
}

func (s *MemoryStore) MarkInventoryExported(_ context.Context, hash string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastInventoryHash = hash
	s.lastInventorySyncTS = at
	return nil
}

func (s *MemoryStore) LastExportedInventoryHash(_ context.Context) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastInventoryHash, nil
}

func (s *MemoryStore) PutAttestation(_ context.Context, result *attestationloop.Result) error {
	if result == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := *result
	s.attestation = &cloned
	s.hasAttestation = true
	return nil
}

func (s *MemoryStore) GetAttestation(_ context.Context) (*attestationloop.Result, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.hasAttestation || s.attestation == nil {
		return nil, false, nil
	}
	cloned := *s.attestation
	return &cloned, true, nil
}

func (s *MemoryStore) MarkAttestationExported(_ context.Context, key string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exportedAttestationKeys[key] = at
	return nil
}

func (s *MemoryStore) WasAttestationExported(_ context.Context, key string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.exportedAttestationKeys[key]
	return ok, nil
}
