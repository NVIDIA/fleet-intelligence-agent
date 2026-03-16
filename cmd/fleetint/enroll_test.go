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

package main

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/precheck"
)

func TestEnrollCommandPrecheckError(t *testing.T) {
	originalRunPrecheck := runPrecheck
	t.Cleanup(func() {
		runPrecheck = originalRunPrecheck
	})

	runPrecheck = func() (precheck.Result, error) {
		return precheck.Result{}, fmt.Errorf("nvml init failed")
	}

	app := App()
	app.Writer = &bytes.Buffer{}

	err := app.Run([]string{"fleetint", "enroll", "--endpoint", "https://example.com", "--token", "token"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to run precheck")
	assert.Contains(t, err.Error(), "nvml init failed")
}

func TestEnrollCommandBlocksOnFailedPrecheck(t *testing.T) {
	originalRunPrecheck := runPrecheck
	originalPerformEnrollment := performEnrollment
	originalStoreConfig := storeEnrollmentConfig
	t.Cleanup(func() {
		runPrecheck = originalRunPrecheck
		performEnrollment = originalPerformEnrollment
		storeEnrollmentConfig = originalStoreConfig
	})

	enrollmentCalled := false
	runPrecheck = func() (precheck.Result, error) {
		return precheck.Result{
			Checks: []precheck.Check{
				{Name: "gpu-present", Message: "no NVIDIA GPU detected"},
			},
		}, nil
	}
	performEnrollment = func(enrollEndpoint, sakToken string) (string, error) {
		enrollmentCalled = true
		return "jwt-token", nil
	}
	storeEnrollmentConfig = func(enrollEndpoint, metricsEndpoint, logsEndpoint, nonceEndpoint, jwtToken, sakToken string) error {
		return nil
	}

	out := &bytes.Buffer{}
	app := App()
	app.Writer = out

	err := app.Run([]string{"fleetint", "enroll", "--endpoint", "https://example.com", "--token", "token"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "precheck failed")
	assert.Contains(t, out.String(), "Enrollment skipped: precheck failed")
	assert.False(t, enrollmentCalled)
}

func TestEnrollCommandForceBypassesFailedPrecheck(t *testing.T) {
	originalRunPrecheck := runPrecheck
	originalPerformEnrollment := performEnrollment
	originalStoreConfig := storeEnrollmentConfig
	t.Cleanup(func() {
		runPrecheck = originalRunPrecheck
		performEnrollment = originalPerformEnrollment
		storeEnrollmentConfig = originalStoreConfig
	})

	enrollmentCalled := false
	runPrecheck = func() (precheck.Result, error) {
		return precheck.Result{
			Checks: []precheck.Check{
				{Name: "gpu-present", Message: "no NVIDIA GPU detected"},
			},
		}, nil
	}
	performEnrollment = func(enrollEndpoint, sakToken string) (string, error) {
		enrollmentCalled = true
		return "jwt-token", nil
	}
	storeEnrollmentConfig = func(enrollEndpoint, metricsEndpoint, logsEndpoint, nonceEndpoint, jwtToken, sakToken string) error {
		return nil
	}

	app := App()
	app.Writer = &bytes.Buffer{}

	err := app.Run([]string{"fleetint", "enroll", "--endpoint", "https://example.com", "--token", "token", "--force"})

	require.NoError(t, err)
	assert.True(t, enrollmentCalled)
}
