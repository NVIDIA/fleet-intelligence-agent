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

package mem

import (
	"testing"

	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
)

func TestMemFieldsIncludeExpectedECCDeviceCounters(t *testing.T) {
	fieldSet := make(map[dcgm.Short]struct{}, len(memFields))
	for _, field := range memFields {
		fieldSet[field] = struct{}{}
	}

	required := []dcgm.Short{
		dcgm.DCGM_FI_DEV_ECC_DBE_AGG_DEV,
		dcgm.DCGM_FI_DEV_ECC_DBE_VOL_DEV,
		dcgm.DCGM_FI_DEV_ECC_SBE_AGG_DEV,
		dcgm.DCGM_FI_DEV_ECC_SBE_VOL_DEV,
	}

	for _, field := range required {
		if _, ok := fieldSet[field]; !ok {
			t.Errorf("missing expected mem field: %d", field)
		}
	}
}
