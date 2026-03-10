// SPDX-FileCopyrightText: Copyright (c) 2024, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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

package common

import (
	"testing"

	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
)

func TestFormatEnrichedIncidents(t *testing.T) {
	tests := []struct {
		name      string
		prefix    string
		incidents []EnrichedIncident
		want      string
	}{
		{
			name:      "no incidents",
			prefix:    "test prefix",
			incidents: nil,
			want:      "test prefix",
		},
		{
			name:      "empty incidents",
			prefix:    "test prefix",
			incidents: []EnrichedIncident{},
			want:      "test prefix",
		},
		{
			name:   "single incident",
			prefix: "thermal warning",
			incidents: []EnrichedIncident{
				{
					UUID:    "GPU-46a3bbe2-3e87-3dde-b464-a03eba0c21d7",
					Message: "Temperature above threshold",
					Code:    dcgm.DCGM_FR_TEMP_VIOLATION,
				},
			},
			want: "thermal warning - GPU GPU-46a3bbe2-3e87-3dde-b464-a03eba0c21d7: Temperature above threshold (code: 42)",
		},
		{
			name:   "multiple incidents",
			prefix: "memory failure",
			incidents: []EnrichedIncident{
				{
					UUID:    "GPU-46a3bbe2-3e87-3dde-b464-a03eba0c21d7",
					Message: "DBE detected",
					Code:    dcgm.DCGM_FR_VOLATILE_DBE_DETECTED,
				},
				{
					UUID:    "GPU-7b4f2c1a-8d6e-4c5b-9a1f-2e3d4c5a6b7c",
					Message: "Row remap failure",
					Code:    dcgm.DCGM_FR_ROW_REMAP_FAILURE,
				},
			},
			want: "memory failure - GPU GPU-46a3bbe2-3e87-3dde-b464-a03eba0c21d7: DBE detected (code: 4); GPU GPU-7b4f2c1a-8d6e-4c5b-9a1f-2e3d4c5a6b7c: Row remap failure (code: 80)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatIncidents(tt.prefix, tt.incidents)
			if got != tt.want {
				t.Errorf("FormatIncidents() = %q, want %q", got, tt.want)
			}
		})
	}
}
