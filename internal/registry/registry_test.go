// SPDX-FileCopyrightText: Copyright (c) 2025, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAll(t *testing.T) {
	components := All()

	// Verify we get all components
	require.NotEmpty(t, components)

	// Verify all components have required fields
	for _, c := range components {
		assert.NotEmpty(t, c.Name, "Component name should not be empty")
		assert.NotNil(t, c.InitFunc, "Component InitFunc should not be nil")
	}

	// Should have at least 20+ components (GPU, DCGM, System, etc.)
	assert.Greater(t, len(components), 20, "Should have many components registered")
}

func TestGetEnabledComponents(t *testing.T) {
	enabled := GetEnabledComponents()

	// All components should be enabled by default in this codebase
	require.NotEmpty(t, enabled)

	// Verify all enabled components have EnabledByDefault = true
	for _, c := range enabled {
		assert.True(t, c.EnabledByDefault, "Enabled component should have EnabledByDefault=true")
	}

	// GetEnabledComponents should return the same as All() in this implementation
	allComponents := All()
	assert.Equal(t, len(allComponents), len(enabled))
}

func TestGetComponent(t *testing.T) {
	tests := []struct {
		name      string
		compName  string
		expectNil bool
	}{
		{
			name:      "existing_component",
			compName:  "cpu",
			expectNil: false,
		},
		{
			name:      "nonexistent_component",
			compName:  "nonexistent-component",
			expectNil: true,
		},
		{
			name:      "empty_name",
			compName:  "",
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			component := GetComponent(tt.compName)
			if tt.expectNil {
				assert.Nil(t, component)
			} else {
				require.NotNil(t, component)
				assert.Equal(t, tt.compName, component.Name)
				assert.NotNil(t, component.InitFunc)
			}
		})
	}
}

func TestComponentNames(t *testing.T) {
	components := All()

	// Check for some expected component names
	expectedNames := []string{
		"accelerator-nvidia-infiniband",
		"accelerator-nvidia-nccl",
		"accelerator-nvidia-dcgm-nvlink",
		"cpu",
		"disk",
		"memory",
	}

	componentNames := make(map[string]bool)
	for _, c := range components {
		componentNames[c.Name] = true
	}

	for _, expectedName := range expectedNames {
		assert.True(t, componentNames[expectedName], "Expected component %s should exist", expectedName)
	}
}

func TestComponentStruct(t *testing.T) {
	// Test Component struct fields
	comp := Component{
		Name:             "test-component",
		InitFunc:         nil,
		EnabledByDefault: true,
	}

	assert.Equal(t, "test-component", comp.Name)
	assert.Nil(t, comp.InitFunc)
	assert.True(t, comp.EnabledByDefault)
}

func TestAllComponentsHaveUniqueNames(t *testing.T) {
	components := All()
	names := make(map[string]bool)

	for _, c := range components {
		assert.False(t, names[c.Name], "Component name %s should be unique", c.Name)
		names[c.Name] = true
	}
}

func TestRemovedComponentsAbsent(t *testing.T) {
	removed := []string{
		"accelerator-nvidia-fabric-manager",
		"accelerator-nvidia-gpu-counts",
		"accelerator-nvidia-nvlink",
		"accelerator-nvidia-dcgm-xid",
		"kernel-module",
		"network-latency",
		"pci",
	}

	for _, name := range removed {
		t.Run(name, func(t *testing.T) {
			assert.Nil(t, GetComponent(name))
		})
	}
}
