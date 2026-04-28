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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestComputeHashIgnoresCollectedAtAndExistingHash(t *testing.T) {
	base := &Snapshot{
		CollectedAt:   time.Unix(100, 0).UTC(),
		InventoryHash: "old-hash",
		Hostname:      "host-a",
		MachineID:     "machine-id",
		Resources: Resources{
			CPUInfo: CPUInfo{Type: "Xeon", LogicalCores: 64},
		},
	}
	other := *base
	other.CollectedAt = time.Unix(200, 0).UTC()
	other.InventoryHash = "different-old-hash"

	hash1, err := ComputeHash(base)
	require.NoError(t, err)
	hash2, err := ComputeHash(&other)
	require.NoError(t, err)
	require.Equal(t, hash1, hash2)

	other.Hostname = "host-b"
	hash3, err := ComputeHash(&other)
	require.NoError(t, err)
	require.NotEqual(t, hash1, hash3)
}
