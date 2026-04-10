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

package precheck

import (
	"fmt"
	"testing"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/machineinfo"
)

func TestEvaluateArchitecture(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        Input
		wantPassed   bool
		wantMessages []string
	}{
		{
			name: "passes for hopper",
			input: Input{
				MachineInfo: machineInfoWithGPU("Hopper", "575.57.08"),
			},
			wantPassed: true,
		},
		{
			name: "passes for blackwell",
			input: Input{
				MachineInfo: machineInfoWithGPU("Blackwell", "575.57.08"),
			},
			wantPassed: true,
		},
		{
			name: "passes for rubin",
			input: Input{
				MachineInfo: machineInfoWithGPU("Rubin", "575.57.08"),
			},
			wantPassed: true,
		},
		{
			name: "fails for missing gpu",
			input: Input{
				MachineInfo: &machineinfo.MachineInfo{},
			},
			wantPassed: false,
			wantMessages: []string{
				"no NVIDIA GPU detected",
			},
		},
		{
			name: "passes for hopper lowercase",
			input: Input{
				MachineInfo: machineInfoWithGPU("hopper", "575.57.08"),
			},
			wantPassed: true,
		},
		{
			name: "fails for unsupported architecture",
			input: Input{
				MachineInfo: machineInfoWithGPU("Ampere", "575.57.08"),
			},
			wantPassed: false,
			wantMessages: []string{
				"unsupported GPU architecture: Ampere",
			},
		},
		{
			name: "fails for empty architecture",
			input: Input{
				MachineInfo: machineInfoWithGPU("", "575.57.08"),
			},
			wantPassed: false,
			wantMessages: []string{
				"GPU architecture is unknown",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := Evaluate(tt.input)

			assert.Equal(t, tt.wantPassed, result.Passed())
			for _, wantMessage := range tt.wantMessages {
				assert.Contains(t, checkMessages(result.Checks), wantMessage)
			}
		})
	}
}

func TestSupportedArchitectures(t *testing.T) {
	t.Parallel()

	require.ElementsMatch(t, []string{"Hopper", "Blackwell", "Rubin"}, SupportedArchitectures())
}

func TestEvaluateDriverAndNVAT(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        Input
		wantPassed   bool
		wantMessages []string
	}{
		{
			name: "fails for missing driver",
			input: Input{
				MachineInfo:     machineInfoWithGPU("Hopper", ""),
				NVAttestPresent: boolPtr(true),
			},
			wantPassed: false,
			wantMessages: []string{
				"NVIDIA driver not detected",
			},
		},
		{
			name: "fails for malformed driver version",
			input: Input{
				MachineInfo:     machineInfoWithGPU("Hopper", "not-a-version"),
				NVAttestPresent: boolPtr(true),
			},
			wantPassed: false,
			wantMessages: []string{
				"failed to parse NVIDIA driver version: failed to parse driver version (expected at least 2 parts): not-a-version",
			},
		},
		{
			name: "fails for driver below minimum major version",
			input: Input{
				MachineInfo:     machineInfoWithGPU("Hopper", "509.12.01"),
				NVAttestPresent: boolPtr(true),
			},
			wantPassed: false,
			wantMessages: []string{
				"NVIDIA driver major version 509 is below required minimum 510",
			},
		},
		{
			name: "fails for missing nvattest",
			input: Input{
				MachineInfo:     machineInfoWithGPU("Hopper", "575.57.08"),
				NVAttestPresent: boolPtr(false),
			},
			wantPassed: false,
			wantMessages: []string{
				"nvattest not found in PATH",
			},
		},
		{
			name: "passes when driver major is at minimum and nvattest is present",
			input: Input{
				MachineInfo:     machineInfoWithGPU("Hopper", "510.47.03"),
				NVAttestPresent: boolPtr(true),
			},
			wantPassed: true,
		},
		{
			name: "passes when newer driver and nvattest are present",
			input: Input{
				MachineInfo:     machineInfoWithGPU("Hopper", "575.57.08"),
				NVAttestPresent: boolPtr(true),
			},
			wantPassed: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := Evaluate(tt.input)

			assert.Equal(t, tt.wantPassed, result.Passed())
			for _, wantMessage := range tt.wantMessages {
				assert.Contains(t, checkMessages(result.Checks), wantMessage)
			}
		})
	}
}

func TestEvaluateAggregatesFailures(t *testing.T) {
	t.Parallel()

	result := Evaluate(Input{
		MachineInfo:     machineInfoWithGPU("Ampere", ""),
		NVAttestPresent: boolPtr(false),
	})

	assert.False(t, result.Passed())
	assert.Contains(t, checkMessages(result.Checks), "unsupported GPU architecture: Ampere")
	assert.Contains(t, checkMessages(result.Checks), "NVIDIA driver not detected")
	assert.Contains(t, checkMessages(result.Checks), "nvattest not found in PATH")
}

func TestEvaluateDCGM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        Input
		wantPassed   bool
		wantMessages []string
	}{
		{
			name: "fails when dcgm is unreachable",
			input: Input{
				MachineInfo:     machineInfoWithGPU("Hopper", "575.57.08"),
				NVAttestPresent: boolPtr(true),
				DCGMReachable:   boolPtr(false),
			},
			wantPassed: false,
			wantMessages: []string{
				"DCGM HostEngine is not reachable",
			},
		},
		{
			name: "fails when dcgm version is too old",
			input: Input{
				MachineInfo:     machineInfoWithGPU("Hopper", "575.57.08"),
				NVAttestPresent: boolPtr(true),
				DCGMReachable:   boolPtr(true),
				DCGMVersion:     "4.2.2",
			},
			wantPassed: false,
			wantMessages: []string{
				"DCGM HostEngine version 4.2.2 is below required minimum 4.2.3",
			},
		},
		{
			name: "passes for minimum supported dcgm version",
			input: Input{
				MachineInfo:     machineInfoWithGPU("Hopper", "575.57.08"),
				NVAttestPresent: boolPtr(true),
				DCGMReachable:   boolPtr(true),
				DCGMVersion:     "4.2.3",
			},
			wantPassed: true,
		},
		{
			name: "passes for newer dcgm version",
			input: Input{
				MachineInfo:     machineInfoWithGPU("Hopper", "575.57.08"),
				NVAttestPresent: boolPtr(true),
				DCGMReachable:   boolPtr(true),
				DCGMVersion:     "4.3.0",
			},
			wantPassed: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := Evaluate(tt.input)

			assert.Equal(t, tt.wantPassed, result.Passed())
			for _, wantMessage := range tt.wantMessages {
				assert.Contains(t, checkMessages(result.Checks), wantMessage)
			}
		})
	}
}

func TestEvaluateDCGMSkipsWhenReachabilityUnset(t *testing.T) {
	t.Parallel()

	result := Evaluate(Input{
		MachineInfo:     machineInfoWithGPU("Hopper", "575.57.08"),
		NVAttestPresent: boolPtr(true),
	})

	assert.True(t, result.Passed())
	assert.Contains(t, checkMessages(result.Checks), "DCGM checks skipped")
}

func TestCollectInputCallsDCGMInit(t *testing.T) {
	originalNewNVML := newNVML
	originalLookPath := lookPath
	originalDCGMInit := dcgmInit
	originalGetHostengineVersion := getHostengineVersion
	originalGetenv := getenv
	t.Cleanup(func() {
		newNVML = originalNewNVML
		lookPath = originalLookPath
		dcgmInit = originalDCGMInit
		getHostengineVersion = originalGetHostengineVersion
		getenv = originalGetenv
	})

	newNVML = func() (nvmlInstance, error) {
		return nil, fmt.Errorf("skip nvml in test")
	}
	lookPath = func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}
	getenv = func(string) string {
		return ""
	}

	dcgmInitCalled := false
	dcgmInit = func() (func(), error) {
		dcgmInitCalled = true
		return func() {}, nil
	}
	getHostengineVersion = func() (string, error) {
		return "4.2.3", nil
	}

	input, err := CollectInput()

	require.NoError(t, err)
	require.NotNil(t, input.DCGMReachable)
	assert.True(t, *input.DCGMReachable)
	assert.Equal(t, "4.2.3", input.DCGMVersion)
	assert.True(t, dcgmInitCalled)
}

func machineInfoWithGPU(architecture, driverVersion string) *machineinfo.MachineInfo {
	return &machineinfo.MachineInfo{
		GPUDriverVersion: driverVersion,
		GPUInfo: &apiv1.MachineGPUInfo{
			Architecture: architecture,
			Product:      "test-gpu",
			GPUs: []apiv1.MachineGPUInstance{
				{UUID: "GPU-1"},
			},
		},
	}
}

func checkMessages(checks []Check) []string {
	messages := make([]string, 0, len(checks))
	for _, check := range checks {
		messages = append(messages, check.Message)
	}
	return messages
}

func boolPtr(v bool) *bool {
	return &v
}
