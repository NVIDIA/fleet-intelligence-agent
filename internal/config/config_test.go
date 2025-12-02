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

package config

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

		assert.True(t, cfg.ShouldDisable("disk"))
		assert.True(t, cfg.ShouldDisable("network"))
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

	t.Run("all with exclusions", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"all", "-kubelet", "-docker", "-tailscale", "-accelerator-nvidia-gsp-firmware"},
		}

		// Should enable all components
		assert.True(t, cfg.ShouldEnable("gpu-memory"))
		assert.True(t, cfg.ShouldEnable("cpu"))
		assert.True(t, cfg.ShouldEnable("kubelet"))
		assert.True(t, cfg.ShouldEnable("docker"))

		// Should disable the excluded components
		assert.True(t, cfg.ShouldDisable("kubelet"))
		assert.True(t, cfg.ShouldDisable("docker"))
		assert.True(t, cfg.ShouldDisable("tailscale"))
		assert.True(t, cfg.ShouldDisable("accelerator-nvidia-gsp-firmware"))

		// Should not disable non-excluded components
		assert.False(t, cfg.ShouldDisable("gpu-memory"))
		assert.False(t, cfg.ShouldDisable("cpu"))
	})
}

func TestDefaultStateFile(t *testing.T) {
	path, err := DefaultStateFile()
	require.NoError(t, err)
	assert.NotEmpty(t, path)
	assert.Contains(t, path, "gpuhealth.state")
}

func TestValidateHealthExporter(t *testing.T) {
	t.Run("valid health check interval", func(t *testing.T) {
		cfg := &Config{
			Address:         ":8080",
			RetentionPeriod: metav1.Duration{Duration: time.Hour},
			HealthExporter: &HealthExporterConfig{
				HealthCheckInterval: metav1.Duration{Duration: 5 * time.Minute},
			},
		}

		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("health check interval too short", func(t *testing.T) {
		cfg := &Config{
			Address:         ":8080",
			RetentionPeriod: metav1.Duration{Duration: time.Hour},
			HealthExporter: &HealthExporterConfig{
				HealthCheckInterval: metav1.Duration{Duration: 500 * time.Millisecond},
			},
		}

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "health_check_interval must be at least 1 second")
	})

	t.Run("health check interval too long", func(t *testing.T) {
		cfg := &Config{
			Address:         ":8080",
			RetentionPeriod: metav1.Duration{Duration: time.Hour},
			HealthExporter: &HealthExporterConfig{
				HealthCheckInterval: metav1.Duration{Duration: 25 * time.Hour},
			},
		}

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "health_check_interval must be at most 24 hours")
	})

	t.Run("health check interval at minimum boundary", func(t *testing.T) {
		cfg := &Config{
			Address:         ":8080",
			RetentionPeriod: metav1.Duration{Duration: time.Hour},
			HealthExporter: &HealthExporterConfig{
				HealthCheckInterval: metav1.Duration{Duration: time.Second},
			},
		}

		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("health check interval at maximum boundary", func(t *testing.T) {
		cfg := &Config{
			Address:         ":8080",
			RetentionPeriod: metav1.Duration{Duration: time.Hour},
			HealthExporter: &HealthExporterConfig{
				HealthCheckInterval: metav1.Duration{Duration: 24 * time.Hour},
			},
		}

		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("health check interval zero (not set)", func(t *testing.T) {
		cfg := &Config{
			Address:         ":8080",
			RetentionPeriod: metav1.Duration{Duration: time.Hour},
			HealthExporter: &HealthExporterConfig{
				HealthCheckInterval: metav1.Duration{Duration: 0},
			},
		}

		err := cfg.Validate()
		assert.NoError(t, err)
	})
}

func TestValidateOfflineMode(t *testing.T) {
	t.Run("valid offline mode configuration", func(t *testing.T) {
		cfg := &Config{
			Address:         ":8080",
			RetentionPeriod: metav1.Duration{Duration: time.Hour},
			HealthExporter: &HealthExporterConfig{
				OfflineMode: true,
				OutputPath:  "/tmp/output",
				Duration:    10 * time.Minute,
			},
		}

		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("offline mode with json format", func(t *testing.T) {
		cfg := &Config{
			Address:         ":8080",
			RetentionPeriod: metav1.Duration{Duration: time.Hour},
			HealthExporter: &HealthExporterConfig{
				OfflineMode:  true,
				OutputPath:   "/tmp/output",
				Duration:     10 * time.Minute,
				OutputFormat: "json",
			},
		}

		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("offline mode with csv format", func(t *testing.T) {
		cfg := &Config{
			Address:         ":8080",
			RetentionPeriod: metav1.Duration{Duration: time.Hour},
			HealthExporter: &HealthExporterConfig{
				OfflineMode:  true,
				OutputPath:   "/tmp/output",
				Duration:     10 * time.Minute,
				OutputFormat: "csv",
			},
		}

		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("offline mode missing output path", func(t *testing.T) {
		cfg := &Config{
			Address:         ":8080",
			RetentionPeriod: metav1.Duration{Duration: time.Hour},
			HealthExporter: &HealthExporterConfig{
				OfflineMode: true,
				Duration:    10 * time.Minute,
			},
		}

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "offline mode: output_path is required")
	})

	t.Run("offline mode missing duration", func(t *testing.T) {
		cfg := &Config{
			Address:         ":8080",
			RetentionPeriod: metav1.Duration{Duration: time.Hour},
			HealthExporter: &HealthExporterConfig{
				OfflineMode: true,
				OutputPath:  "/tmp/output",
			},
		}

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "offline mode: duration is required")
	})

	t.Run("offline mode with invalid output format", func(t *testing.T) {
		cfg := &Config{
			Address:         ":8080",
			RetentionPeriod: metav1.Duration{Duration: time.Hour},
			HealthExporter: &HealthExporterConfig{
				OfflineMode:  true,
				OutputPath:   "/tmp/output",
				Duration:     10 * time.Minute,
				OutputFormat: "xml",
			},
		}

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "offline mode: output_format must be 'json' or 'csv'")
	})

	t.Run("offline mode allows missing address", func(t *testing.T) {
		cfg := &Config{
			Address:         "",
			RetentionPeriod: metav1.Duration{Duration: time.Hour},
			HealthExporter: &HealthExporterConfig{
				OfflineMode: true,
				OutputPath:  "/tmp/output",
				Duration:    10 * time.Minute,
			},
		}

		err := cfg.Validate()
		assert.NoError(t, err)
	})
}

func TestValidateRetentionPeriod(t *testing.T) {
	t.Run("retention period at minimum boundary", func(t *testing.T) {
		cfg := &Config{
			Address:         ":8080",
			RetentionPeriod: metav1.Duration{Duration: time.Minute},
		}

		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("retention period zero", func(t *testing.T) {
		cfg := &Config{
			Address:         ":8080",
			RetentionPeriod: metav1.Duration{Duration: 0},
		}

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "retention_period must be at least 1 minute")
	})

	t.Run("retention period negative", func(t *testing.T) {
		cfg := &Config{
			Address:         ":8080",
			RetentionPeriod: metav1.Duration{Duration: -1 * time.Hour},
		}

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "retention_period must be at least 1 minute")
	})
}

func TestComponentSelectionEdgeCases(t *testing.T) {
	t.Run("empty string component", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"", "gpu-memory"},
		}

		// Empty strings should be ignored
		assert.True(t, cfg.ShouldEnable("gpu-memory"))
		assert.False(t, cfg.ShouldEnable("cpu"))
	})

	t.Run("multiple wildcards", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"*", "*"},
		}

		assert.True(t, cfg.ShouldEnable("gpu-memory"))
		assert.True(t, cfg.ShouldEnable("cpu"))
	})

	t.Run("all and wildcard mixed", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"all", "*"},
		}

		assert.True(t, cfg.ShouldEnable("gpu-memory"))
		assert.True(t, cfg.ShouldEnable("any-component"))
	})

	t.Run("specific components then wildcard", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"gpu-memory", "*"},
		}

		// First call initializes based on first element
		assert.True(t, cfg.ShouldEnable("gpu-memory"))
		// Cache is initialized after first call
		assert.False(t, cfg.ShouldEnable("cpu"))
	})

	t.Run("disable component with hyphen in name", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"-gpu-memory"},
		}

		assert.True(t, cfg.ShouldDisable("gpu-memory"))
		assert.False(t, cfg.ShouldDisable("cpu"))
	})

	t.Run("enable and disable mixed", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"gpu-memory", "-cpu"},
		}

		// Should enable gpu-memory
		assert.True(t, cfg.ShouldEnable("gpu-memory"))
		// Should not enable cpu (not in enabled list)
		assert.False(t, cfg.ShouldEnable("cpu"))
		// Should disable cpu (in disabled list)
		assert.True(t, cfg.ShouldDisable("cpu"))
	})

	t.Run("wildcard with single exclusion", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"*", "-kubelet"},
		}

		assert.True(t, cfg.ShouldEnable("gpu-memory"))
		assert.True(t, cfg.ShouldEnable("kubelet"))
		assert.True(t, cfg.ShouldDisable("kubelet"))
	})

	t.Run("ShouldDisable with wildcard", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"*"},
		}

		// Wildcard doesn't disable anything
		assert.False(t, cfg.ShouldDisable("gpu-memory"))
		assert.False(t, cfg.ShouldDisable("cpu"))
	})

	t.Run("ShouldDisable with all keyword", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"all"},
		}

		// "all" doesn't disable anything
		assert.False(t, cfg.ShouldDisable("gpu-memory"))
		assert.False(t, cfg.ShouldDisable("cpu"))
	})
}

func TestDefaultWithHealthExporter(t *testing.T) {
	ctx := context.Background()

	t.Run("default includes health exporter", func(t *testing.T) {
		cfg, err := Default(ctx)
		require.NoError(t, err)

		assert.NotNil(t, cfg.HealthExporter)
		assert.False(t, cfg.HealthExporter.AttestationEnabled) // Attestation should be disabled by default
		assert.Equal(t, metav1.Duration{Duration: 24 * time.Hour}, cfg.HealthExporter.AttestationInterval)
		assert.Equal(t, metav1.Duration{Duration: 1 * time.Minute}, cfg.HealthExporter.Interval)
		assert.Equal(t, metav1.Duration{Duration: 30 * time.Second}, cfg.HealthExporter.Timeout)
		assert.True(t, cfg.HealthExporter.IncludeMetrics)
		assert.True(t, cfg.HealthExporter.IncludeEvents)
		assert.True(t, cfg.HealthExporter.IncludeMachineInfo)
		assert.True(t, cfg.HealthExporter.IncludeComponentData)
		assert.Equal(t, metav1.Duration{Duration: 1 * time.Minute}, cfg.HealthExporter.MetricsLookback)
		assert.Equal(t, metav1.Duration{Duration: 1 * time.Minute}, cfg.HealthExporter.EventsLookback)
		assert.Equal(t, metav1.Duration{Duration: 1 * time.Minute}, cfg.HealthExporter.HealthCheckInterval)
		assert.Equal(t, 3, cfg.HealthExporter.RetryMaxAttempts)
		assert.Equal(t, "json", cfg.HealthExporter.OutputFormat)
		assert.Equal(t, "", cfg.HealthExporter.MetricsEndpoint)
		assert.Equal(t, "", cfg.HealthExporter.LogsEndpoint)
		assert.Equal(t, "", cfg.HealthExporter.AuthToken)
		assert.False(t, cfg.HealthExporter.OfflineMode)
	})
}
