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

package inventory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Manager coordinates periodic inventory collection into a store.
type Manager interface {
	Run(ctx context.Context) error
	CollectOnce(ctx context.Context) (*Snapshot, error)
}

type manager struct {
	mu       sync.RWMutex
	source   Source
	sink     Sink
	interval time.Duration

	lastSnapshot     *Snapshot
	lastExportedHash string
}

// NewManager creates an inventory manager.
func NewManager(source Source, sink Sink, interval time.Duration) Manager {
	return &manager{
		source:   source,
		sink:     sink,
		interval: interval,
	}
}

func (m *manager) Run(ctx context.Context) error {
	if _, err := m.CollectOnce(ctx); err != nil {
		return err
	}

	if m.interval <= 0 {
		return nil
	}

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := m.CollectOnce(ctx); err != nil {
				return err
			}
		}
	}
}

func (m *manager) CollectOnce(ctx context.Context) (*Snapshot, error) {
	if m.source == nil {
		return nil, fmt.Errorf("inventory source is required")
	}
	snap, err := m.source.Collect(ctx)
	if err != nil {
		return nil, err
	}
	if snap == nil {
		return nil, fmt.Errorf("inventory source returned nil snapshot")
	}
	hash, err := ComputeHash(snap)
	if err != nil {
		return nil, err
	}
	snap.InventoryHash = hash

	m.mu.Lock()
	cloned := *snap
	m.lastSnapshot = &cloned
	shouldExport := m.sink != nil && m.lastExportedHash != hash
	m.mu.Unlock()

	if shouldExport {
		if err := m.sink.Export(ctx, snap); err != nil {
			if errors.Is(err, ErrNotReady) {
				return snap, nil
			}
			return nil, err
		}
		m.mu.Lock()
		m.lastExportedHash = hash
		m.mu.Unlock()
	}

	return snap, nil
}
