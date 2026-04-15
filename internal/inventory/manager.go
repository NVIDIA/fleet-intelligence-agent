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
	"fmt"
	"time"
)

// Manager coordinates periodic inventory collection into a store.
type Manager interface {
	Run(ctx context.Context) error
	CollectOnce(ctx context.Context) (*Snapshot, error)
}

type manager struct {
	source   Source
	store    StateStore
	interval time.Duration
}

// NewManager creates an inventory manager skeleton.
func NewManager(source Source, store StateStore, interval time.Duration) Manager {
	return &manager{
		source:   source,
		store:    store,
		interval: interval,
	}
}

func (m *manager) Run(ctx context.Context) error {
	if _, err := m.CollectOnce(ctx); err != nil {
		return err
	}
	return fmt.Errorf("inventory manager run loop not implemented")
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
	if m.store != nil {
		if err := m.store.PutInventory(ctx, snap); err != nil {
			return nil, err
		}
	}
	return snap, nil
}
