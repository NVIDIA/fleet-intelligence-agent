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
	"errors"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgconfigcommon "github.com/leptonai/gpud/pkg/config/common"
)

// Config provides configuration for the health metrics exporter
type Config struct {
	// APIVersion for the health server
	APIVersion string `json:"api_version"`

	// Address for the health server to listen on
	Address string `json:"address"`

	// State file that persists health status and metrics
	// If empty, states are not persisted to file
	State string `json:"state"`

	// Amount of time to retain health states/metrics
	// Once elapsed, old data is automatically purged
	RetentionPeriod metav1.Duration `json:"retention_period"`

	// NVIDIA tool command paths to overwrite defaults
	NvidiaToolOverwrites pkgconfigcommon.ToolOverwrites `json:"nvidia_tool_overwrites"`

	// Components specifies which health components to enable
	// Leave empty, "*", or "all" to enable all components
	// Prefix component names with "-" to disable them
	Components         []string       `json:"components"`
	selectedComponents map[string]any `json:"-"`
	disabledComponents map[string]any `json:"-"`

	// Health Exporter Configuration
	HealthExporter *HealthExporterConfig `json:"health_exporter,omitempty"`
}

// HealthExporterConfig holds configuration for the health data exporter
type HealthExporterConfig struct {
	// MetricsEndpoint is the specific endpoint for sending metrics data
	MetricsEndpoint string `json:"metrics_endpoint"`

	// LogsEndpoint is the specific endpoint for sending logs/events data
	LogsEndpoint string `json:"logs_endpoint"`

	// AttestationEnabled controls whether attestation functionality is enabled
	AttestationEnabled bool `json:"attestation_enabled"`

	// AttestationInterval is how often to run attestation (default: 24 hours)
	AttestationInterval metav1.Duration `json:"attestation_interval"`

	// AuthToken is the authentication token for HTTP requests
	AuthToken string `json:"auth_token,omitempty"`

	// Interval is how often to export health data
	Interval metav1.Duration `json:"interval"`

	// Timeout for HTTP requests to the global health endpoint
	Timeout metav1.Duration `json:"timeout"`

	// IncludeMetrics controls whether to include metrics data in exports
	IncludeMetrics bool `json:"include_metrics"`

	// IncludeEvents controls whether to include events data in exports
	IncludeEvents bool `json:"include_events"`

	// IncludeMachineInfo controls whether to include machine hardware info in exports
	IncludeMachineInfo bool `json:"include_machine_info"`

	// IncludeComponentData controls whether to include actual component data/numbers in exports
	IncludeComponentData bool `json:"include_component_data"`

	// MetricsLookback determines how far back to look for metrics data
	MetricsLookback metav1.Duration `json:"metrics_lookback"`

	// EventsLookback determines how far back to look for events data
	EventsLookback metav1.Duration `json:"events_lookback"`

	// HealthCheckInterval determines how often individual components perform their health checks
	// Valid range: 1 second (minimum) to 24 hours (maximum), default is 1 minute
	HealthCheckInterval metav1.Duration `json:"health_check_interval"`

	// RetryMaxAttempts is the maximum number of retry attempts for failed requests
	RetryMaxAttempts int `json:"retry_max_attempts"`

	// Offline mode configuration
	// OfflineMode controls whether to use offline mode (write to files instead of HTTP endpoint)
	OfflineMode bool `json:"offline_mode"`

	// OutputPath is the directory path where files will be written (required when OfflineMode is true)
	OutputPath string `json:"output_path"`

	// OutputFormat specifies the format for offline mode output files: "json" (default) or "csv"
	OutputFormat string `json:"output_format"`

	// Duration is how long to collect telemetry data in offline mode
	Duration time.Duration `json:"duration"`
}

// Validate checks if the configuration is valid
func (config *Config) Validate() error {
	// In offline mode, address is not required
	isOfflineMode := config.HealthExporter != nil && config.HealthExporter.OfflineMode
	if !isOfflineMode {
		if config.Address == "" {
			return errors.New("address is required")
		}
	}

	if config.RetentionPeriod.Duration < time.Minute {
		return fmt.Errorf("retention_period must be at least 1 minute, got %v", config.RetentionPeriod.Duration)
	}

	// Validate health exporter configuration if present
	if config.HealthExporter != nil {
		// Validate health check interval
		if config.HealthExporter.HealthCheckInterval.Duration != 0 {
			if config.HealthExporter.HealthCheckInterval.Duration < time.Second {
				return fmt.Errorf("health_check_interval must be at least 1 second, got %v", config.HealthExporter.HealthCheckInterval.Duration)
			}
			if config.HealthExporter.HealthCheckInterval.Duration > 24*time.Hour {
				return fmt.Errorf("health_check_interval must be at most 24 hours, got %v", config.HealthExporter.HealthCheckInterval.Duration)
			}
		}

		// Validate offline mode configuration if present
		if config.HealthExporter.OfflineMode {
			if config.HealthExporter.OutputPath == "" {
				return errors.New("offline mode: output_path is required")
			}
			if config.HealthExporter.Duration <= 0 {
				return errors.New("offline mode: duration is required")
			}
			// Validate output format
			if config.HealthExporter.OutputFormat != "" {
				if config.HealthExporter.OutputFormat != "json" && config.HealthExporter.OutputFormat != "csv" {
					return fmt.Errorf("offline mode: output_format must be 'json' or 'csv', got '%s'", config.HealthExporter.OutputFormat)
				}
			}
		}
	}

	return nil
}

// ShouldEnable returns true if the component should be enabled.
// If no components are specified, all components are enabled by default.
func (config *Config) ShouldEnable(componentName string) bool {
	// Not specified, enable all components
	if len(config.Components) == 0 || config.Components[0] == "*" || config.Components[0] == "all" {
		return true
	}

	if config.selectedComponents == nil {
		config.selectedComponents = make(map[string]any)

		for _, c := range config.Components {
			if c == "*" || c == "all" {
				// Enable all components
				return true
			}

			// Prefix "-" is used to disable a component
			if strings.HasPrefix(c, "-") {
				continue
			}
			config.selectedComponents[c] = struct{}{}
		}
	}

	_, shouldEnable := config.selectedComponents[componentName]
	return shouldEnable
}

// ShouldDisable returns true if the component should be disabled.
// If no disable components are specified, all components are enabled by default.
func (config *Config) ShouldDisable(componentName string) bool {
	// Not specified, enable all components (don't disable any)
	if len(config.Components) == 0 {
		return false
	}

	if config.disabledComponents == nil {
		config.disabledComponents = make(map[string]any)

		for _, c := range config.Components {
			// Skip "all" and "*" markers
			if c == "*" || c == "all" {
				continue
			}

			// Prefix "-" is used to disable a component
			if strings.HasPrefix(c, "-") {
				// Store without the "-" prefix for matching
				config.disabledComponents[strings.TrimPrefix(c, "-")] = struct{}{}
			}
		}
	}

	_, shouldDisable := config.disabledComponents[componentName]
	return shouldDisable
}
