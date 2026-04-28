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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// ComputeHash returns a deterministic hash for the stable inventory contents.
func ComputeHash(snap *Snapshot) (string, error) {
	if snap == nil {
		return "", fmt.Errorf("inventory snapshot is nil")
	}
	normalized := *snap
	normalized.CollectedAt = time.Time{}
	normalized.InventoryHash = ""

	payload, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal inventory snapshot: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
