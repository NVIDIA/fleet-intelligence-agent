// SPDX-FileCopyrightText: Copyright (c) 2025, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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

package nvml

import (
	"context"
	"testing"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	"github.com/NVIDIA/fleet-intelligence-sdk/components"
	nvidianvml "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/nvml"
	"github.com/stretchr/testify/require"
)

func TestRunCheckWithErrorsUpdatesState(t *testing.T) {
	c, err := New(&components.GPUdInstance{
		RootCtx:      context.Background(),
		NVMLInstance: &mockNVMLInstance{},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	RunCheckWithErrors([]string{"gpu GPU-1: get_memory failed: GPU is lost"})
	states := c.LastHealthStates()
	require.Len(t, states, 1)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	require.Len(t, states[0].Incidents, 1)
	require.Equal(t, "GPU-1", states[0].Incidents[0].EntityID)

	RunCheckWithErrors(nil)
	states = c.LastHealthStates()
	require.Len(t, states, 1)
	require.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
}

type mockNVMLInstance struct {
	nvidianvml.Instance
}

func (m *mockNVMLInstance) NVMLExists() bool { return true }
