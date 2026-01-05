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
	"fmt"
	stdos "os"
	"path/filepath"
	"time"

	"github.com/mitchellh/go-homedir"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nvidiacommon "github.com/leptonai/gpud/pkg/config/common"
)

const (
	// DefaultAPIVersion for health server
	DefaultAPIVersion = "v1"

	// DefaultHealthPort for health metrics export
	DefaultHealthPort = 15133
)

var (
	// DefaultRetentionPeriod - keep health data for 4 hours by default
	DefaultRetentionPeriod = metav1.Duration{Duration: 4 * time.Hour}
)

// Default creates a default health configuration
func Default(ctx context.Context, opts ...OpOption) (*Config, error) {
	options := &Op{}
	if err := options.ApplyOpts(opts); err != nil {
		return nil, err
	}

	cfg := &Config{
		APIVersion:      DefaultAPIVersion,
		Address:         fmt.Sprintf(":%d", DefaultHealthPort),
		RetentionPeriod: DefaultRetentionPeriod,
		NvidiaToolOverwrites: nvidiacommon.ToolOverwrites{
			InfinibandClassRootDir: options.InfinibandClassRootDir,
		},
		// Health exporter is enabled by default
		HealthExporter: &HealthExporterConfig{
			MetricsEndpoint:      "",
			LogsEndpoint:         "",
			AttestationEnabled:   false,
			AttestationInterval:  metav1.Duration{Duration: 24 * time.Hour}, // Default 24 hours
			AuthToken:            "",
			Interval:             metav1.Duration{Duration: 1 * time.Minute},
			Timeout:              metav1.Duration{Duration: 30 * time.Second},
			IncludeMetrics:       true,
			IncludeEvents:        true,
			IncludeMachineInfo:   true,
			IncludeComponentData: true,
			MetricsLookback:      metav1.Duration{Duration: 1 * time.Minute},
			EventsLookback:       metav1.Duration{Duration: 1 * time.Minute},
			HealthCheckInterval:  metav1.Duration{Duration: 1 * time.Minute}, // Default 1 minute for component health checks
			RetryMaxAttempts:     3,                                          // Retry up to 3 times
			OutputFormat:         "json",                                     // Default to JSON format for offline mode
		},
	}

	if cfg.State == "" {
		var err error
		cfg.State, err = DefaultStateFile()
		if err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

const defaultVarLibDir = "/var/lib/gpuhealth"

// setupDefaultDir creates the default directory for health data
func setupDefaultDir() (string, error) {
	asRoot := stdos.Geteuid() == 0 // running as root

	d := defaultVarLibDir
	_, err := stdos.Stat("/var/lib")
	if !asRoot || stdos.IsNotExist(err) {
		homeDir, err := homedir.Dir()
		if err != nil {
			return "", err
		}
		d = filepath.Join(homeDir, ".gpuhealth")
	}

	if _, err := stdos.Stat(d); stdos.IsNotExist(err) {
		if err = stdos.MkdirAll(d, 0755); err != nil {
			return "", err
		}
	}
	return d, nil
}

// DefaultStateFile returns the default path for the health state database
func DefaultStateFile() (string, error) {
	dir, err := setupDefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gpuhealth.state"), nil
}
