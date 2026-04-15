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

// Package source contains inventory collection adapters.
package source

import (
	"context"
	"fmt"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/inventory"
)

// MachineInfoCollector is the local machine inventory collector dependency.
type MachineInfoCollector interface {
	Collect(ctx context.Context) (*inventory.Snapshot, error)
}

type machineInfoSource struct {
	collector MachineInfoCollector
}

// NewMachineInfoSource wraps the machine inventory collector as an inventory source.
func NewMachineInfoSource(collector MachineInfoCollector) inventory.Source {
	return &machineInfoSource{collector: collector}
}

func (s *machineInfoSource) Collect(ctx context.Context) (*inventory.Snapshot, error) {
	if s.collector == nil {
		return nil, fmt.Errorf("machine info collector is required")
	}
	return s.collector.Collect(ctx)
}
