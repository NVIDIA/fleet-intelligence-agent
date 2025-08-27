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

package gpuhealthconfig

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDefault(t *testing.T) {
	ctx := context.Background()

	t.Run("default values", func(t *testing.T) {
		cfg, err := Default(ctx)
		require.NoError(t, err)

		// Check basic properties
		assert.Equal(t, DefaultAPIVersion, cfg.APIVersion)
		assert.Equal(t, ":15133", cfg.Address)
		assert.Equal(t, DefaultRetentionPeriod, cfg.RetentionPeriod)
		assert.Equal(t, DefaultCompactPeriod, cfg.CompactPeriod)
		assert.False(t, cfg.Pprof)

		// State path should be set
		assert.NotEmpty(t, cfg.State, "State path should be set")
	})

	t.Run("with infiniband class dir option", func(t *testing.T) {
		classDir := "/custom/class"

		cfg, err := Default(ctx, WithInfinibandClassRootDir(classDir))
		require.NoError(t, err)

		assert.Equal(t, classDir, cfg.NvidiaToolOverwrites.InfinibandClassRootDir)
	})
}

func TestConfigValidation(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := &Config{
			Address:         ":8080",
			RetentionPeriod: metav1.Duration{Duration: time.Hour},
		}

		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing address", func(t *testing.T) {
		cfg := &Config{
			RetentionPeriod: metav1.Duration{Duration: time.Hour},
		}

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "address is required")
	})

	t.Run("retention period too short", func(t *testing.T) {
		cfg := &Config{
			Address:         ":8080",
			RetentionPeriod: metav1.Duration{Duration: 30 * time.Second},
		}

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "retention_period must be at least 1 minute")
	})
}

func TestComponentSelection(t *testing.T) {
	t.Run("enable all components by default", func(t *testing.T) {
		cfg := &Config{}

		assert.True(t, cfg.ShouldEnable("gpu-memory"))
		assert.True(t, cfg.ShouldEnable("cpu"))
		assert.False(t, cfg.ShouldDisable("gpu-memory"))
		assert.False(t, cfg.ShouldDisable("cpu"))
	})

	t.Run("enable specific components", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"gpu-memory", "cpu"},
		}

		assert.True(t, cfg.ShouldEnable("gpu-memory"))
		assert.True(t, cfg.ShouldEnable("cpu"))
		assert.False(t, cfg.ShouldEnable("disk"))
		assert.False(t, cfg.ShouldDisable("gpu-memory"))
	})

	t.Run("disable specific components", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"-disk", "-network"},
		}

		assert.True(t, cfg.ShouldDisable("-disk"))
		assert.True(t, cfg.ShouldDisable("-network"))
		assert.False(t, cfg.ShouldDisable("gpu-memory"))
		assert.False(t, cfg.ShouldEnable("disk"))
	})

	t.Run("wildcard enables all", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"*"},
		}

		assert.True(t, cfg.ShouldEnable("gpu-memory"))
		assert.True(t, cfg.ShouldEnable("any-component"))
		assert.False(t, cfg.ShouldDisable("gpu-memory"))
	})

	t.Run("all keyword enables all", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"all"},
		}

		assert.True(t, cfg.ShouldEnable("gpu-memory"))
		assert.True(t, cfg.ShouldEnable("any-component"))
		assert.False(t, cfg.ShouldDisable("gpu-memory"))
	})
}

func TestDefaultStateFile(t *testing.T) {
	path, err := DefaultStateFile()
	require.NoError(t, err)
	assert.NotEmpty(t, path)
	assert.Contains(t, path, "gpuhealth.state")
}
