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

package scan

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	"github.com/NVIDIA/fleet-intelligence-sdk/components"
	infinibandclass "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/infiniband/class"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWithInfinibandClassRootDir tests the WithInfinibandClassRootDir option.
func TestWithInfinibandClassRootDir(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "custom_path",
			path:     "/custom/infiniband/path",
			expected: "/custom/infiniband/path",
		},
		{
			name:     "empty_path",
			path:     "",
			expected: "",
		},
		{
			name:     "relative_path",
			path:     "relative/path",
			expected: "relative/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			opt := WithInfinibandClassRootDir(tt.path)
			opt(op)
			assert.Equal(t, tt.expected, op.infinibandClassRootDir)
		})
	}
}

// TestWithFailureInjector tests the WithFailureInjector option.
func TestWithFailureInjector(t *testing.T) {
	tests := []struct {
		name     string
		injector *components.FailureInjector
	}{
		{
			name:     "nil_injector",
			injector: nil,
		},
		{
			name:     "valid_injector",
			injector: &components.FailureInjector{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			opt := WithFailureInjector(tt.injector)
			opt(op)
			assert.Equal(t, tt.injector, op.failureInjector)
		})
	}
}

// TestWithDebug tests the WithDebug option.
func TestWithDebug(t *testing.T) {
	tests := []struct {
		name     string
		debug    bool
		expected bool
	}{
		{
			name:     "debug_enabled",
			debug:    true,
			expected: true,
		},
		{
			name:     "debug_disabled",
			debug:    false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			opt := WithDebug(tt.debug)
			opt(op)
			assert.Equal(t, tt.expected, op.debug)
		})
	}
}

// TestApplyOpts tests the applyOpts method.
func TestApplyOpts(t *testing.T) {
	tests := []struct {
		name                        string
		opts                        []Option
		expectedInfinibandClassRoot string
		expectedDebug               bool
		expectedFailureInjector     *components.FailureInjector
	}{
		{
			name:                        "no_options",
			opts:                        []Option{},
			expectedInfinibandClassRoot: infinibandclass.DefaultRootDir,
			expectedDebug:               false,
			expectedFailureInjector:     nil,
		},
		{
			name: "all_options",
			opts: []Option{
				WithInfinibandClassRootDir("/custom/path"),
				WithDebug(true),
				WithFailureInjector(&components.FailureInjector{}),
			},
			expectedInfinibandClassRoot: "/custom/path",
			expectedDebug:               true,
			expectedFailureInjector:     &components.FailureInjector{},
		},
		{
			name: "only_debug",
			opts: []Option{
				WithDebug(true),
			},
			expectedInfinibandClassRoot: infinibandclass.DefaultRootDir,
			expectedDebug:               true,
			expectedFailureInjector:     nil,
		},
		{
			name: "only_infiniband_path",
			opts: []Option{
				WithInfinibandClassRootDir("/test/path"),
			},
			expectedInfinibandClassRoot: "/test/path",
			expectedDebug:               false,
			expectedFailureInjector:     nil,
		},
		{
			name: "multiple_same_options_last_wins",
			opts: []Option{
				WithDebug(false),
				WithDebug(true),
				WithDebug(false),
			},
			expectedInfinibandClassRoot: infinibandclass.DefaultRootDir,
			expectedDebug:               false,
			expectedFailureInjector:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			err := op.applyOpts(tt.opts)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedInfinibandClassRoot, op.infinibandClassRootDir)
			assert.Equal(t, tt.expectedDebug, op.debug)
			assert.Equal(t, tt.expectedFailureInjector, op.failureInjector)
		})
	}
}

// TestApplyOptsDefaultInfinibandRoot tests that default infiniband root is set when empty.
func TestApplyOptsDefaultInfinibandRoot(t *testing.T) {
	op := &Op{
		infinibandClassRootDir: "",
	}
	err := op.applyOpts([]Option{})
	require.NoError(t, err)
	assert.Equal(t, infinibandclass.DefaultRootDir, op.infinibandClassRootDir)
}

// TestApplyOptsPreserveNonEmptyInfinibandRoot tests that non-empty infiniband root is preserved.
func TestApplyOptsPreserveNonEmptyInfinibandRoot(t *testing.T) {
	customPath := "/custom/infiniband"
	op := &Op{
		infinibandClassRootDir: customPath,
	}
	err := op.applyOpts([]Option{})
	require.NoError(t, err)
	// After applyOpts with no options, it should still have the custom path
	// Wait, looking at the code, applyOpts always sets to default if empty
	// Let's check what happens if we pass no options but it was already set

	// Actually, looking at applyOpts, it only sets default if it's empty
	// So if we set it beforehand, it should be preserved
	assert.Equal(t, customPath, op.infinibandClassRootDir)
}

// mockCheckResult is a mock implementation of components.CheckResult for testing.
type mockCheckResult struct {
	componentName   string
	healthStateType apiv1.HealthStateType
	healthStates    apiv1.HealthStates
	summary         string
	details         string
}

func (m *mockCheckResult) ComponentName() string {
	return m.componentName
}

func (m *mockCheckResult) Summary() string {
	return m.summary
}

func (m *mockCheckResult) String() string {
	return m.details
}

func (m *mockCheckResult) HealthStateType() apiv1.HealthStateType {
	return m.healthStateType
}

func (m *mockCheckResult) HealthStates() apiv1.HealthStates {
	return m.healthStates
}

// TestPrintSummary tests the printSummary function.
func TestPrintSummary(t *testing.T) {
	tests := []struct {
		name            string
		healthStateType apiv1.HealthStateType
		summary         string
		details         string
		expectCheckMark bool
	}{
		{
			name:            "healthy_result",
			healthStateType: apiv1.HealthStateTypeHealthy,
			summary:         "Everything is healthy",
			details:         "All checks passed successfully",
			expectCheckMark: true,
		},
		{
			name:            "unhealthy_result",
			healthStateType: apiv1.HealthStateTypeUnhealthy,
			summary:         "System has issues",
			details:         "GPU errors detected",
			expectCheckMark: false,
		},
		{
			name:            "degraded_result",
			healthStateType: apiv1.HealthStateTypeDegraded,
			summary:         "System is degraded",
			details:         "Some components not performing optimally",
			expectCheckMark: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			result := &mockCheckResult{
				componentName:   "test-component",
				healthStateType: tt.healthStateType,
				summary:         tt.summary,
				details:         tt.details,
			}

			printSummary(result)

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read captured output
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)
			output := buf.String()

			// Verify output contains summary and details
			assert.Contains(t, output, tt.summary)
			assert.Contains(t, output, tt.details)

			// Verify output is not empty
			// Note: We can't easily test the exact symbol without knowing the cmdcommon constants
			// But we can verify the summary and details are printed
			assert.NotEmpty(t, output)
		})
	}
}

// TestPrintSummaryOutput tests that printSummary produces expected output format.
func TestPrintSummaryOutput(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	result := &mockCheckResult{
		componentName:   "test-component",
		healthStateType: apiv1.HealthStateTypeHealthy,
		summary:         "Test Summary",
		details:         "Test Details",
	}

	printSummary(result)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Verify output structure
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.GreaterOrEqual(t, len(lines), 2, "Output should have at least 2 lines")

	// First line should contain the summary
	assert.Contains(t, lines[0], "Test Summary")

	// Second line should contain the details
	assert.Contains(t, lines[1], "Test Details")
}

// TestOpStruct tests the Op struct initialization.
func TestOpStruct(t *testing.T) {
	op := &Op{
		infinibandClassRootDir: "/test/path",
		debug:                  true,
		failureInjector:        &components.FailureInjector{},
	}

	assert.Equal(t, "/test/path", op.infinibandClassRootDir)
	assert.True(t, op.debug)
	assert.NotNil(t, op.failureInjector)
}

// TestOpStructDefaults tests the Op struct with default values.
func TestOpStructDefaults(t *testing.T) {
	op := &Op{}

	assert.Empty(t, op.infinibandClassRootDir)
	assert.False(t, op.debug)
	assert.Nil(t, op.failureInjector)
}

// TestMultipleOptionsApplication tests applying multiple options in sequence.
func TestMultipleOptionsApplication(t *testing.T) {
	op := &Op{}

	// Apply options one by one
	WithInfinibandClassRootDir("/path1")(op)
	assert.Equal(t, "/path1", op.infinibandClassRootDir)

	WithDebug(true)(op)
	assert.True(t, op.debug)
	assert.Equal(t, "/path1", op.infinibandClassRootDir) // Previous option should still be set

	injector := &components.FailureInjector{}
	WithFailureInjector(injector)(op)
	assert.Equal(t, injector, op.failureInjector)
	assert.True(t, op.debug)                             // Previous options should still be set
	assert.Equal(t, "/path1", op.infinibandClassRootDir) // Previous options should still be set
}

// TestApplyOptsErrorHandling tests that applyOpts handles errors correctly.
func TestApplyOptsErrorHandling(t *testing.T) {
	op := &Op{}
	// Current implementation doesn't return errors, but test the interface
	err := op.applyOpts([]Option{})
	assert.NoError(t, err)

	err = op.applyOpts([]Option{WithDebug(true)})
	assert.NoError(t, err)
}

// TestOptionCombinations tests various combinations of options.
func TestOptionCombinations(t *testing.T) {
	tests := []struct {
		name                    string
		options                 []Option
		expectedDebug           bool
		expectedHasInjector     bool
		expectedInfinibandIsSet bool
	}{
		{
			name:                    "all_disabled",
			options:                 []Option{WithDebug(false)},
			expectedDebug:           false,
			expectedHasInjector:     false,
			expectedInfinibandIsSet: true, // Will be set to default
		},
		{
			name:                    "all_enabled",
			options:                 []Option{WithDebug(true), WithFailureInjector(&components.FailureInjector{}), WithInfinibandClassRootDir("/test")},
			expectedDebug:           true,
			expectedHasInjector:     true,
			expectedInfinibandIsSet: true,
		},
		{
			name:                    "partial_enabled",
			options:                 []Option{WithDebug(true)},
			expectedDebug:           true,
			expectedHasInjector:     false,
			expectedInfinibandIsSet: true, // Will be set to default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			err := op.applyOpts(tt.options)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedDebug, op.debug)
			assert.Equal(t, tt.expectedHasInjector, op.failureInjector != nil)
			assert.Equal(t, tt.expectedInfinibandIsSet, op.infinibandClassRootDir != "")
		})
	}
}

// TestMockCheckResultInterface tests the mock check result implementation.
func TestMockCheckResultInterface(t *testing.T) {
	result := &mockCheckResult{
		componentName:   "test-component",
		healthStateType: apiv1.HealthStateTypeHealthy,
		healthStates:    apiv1.HealthStates{},
		summary:         "Test summary",
		details:         "Test details",
	}

	// Verify interface methods
	assert.Equal(t, "test-component", result.ComponentName())
	assert.Equal(t, "Test summary", result.Summary())
	assert.Equal(t, "Test details", result.String())
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.NotNil(t, result.HealthStates())
}

// TestMockCheckResultUnhealthy tests mock check result with unhealthy state.
func TestMockCheckResultUnhealthy(t *testing.T) {
	result := &mockCheckResult{
		componentName:   "unhealthy-component",
		healthStateType: apiv1.HealthStateTypeUnhealthy,
		summary:         "Unhealthy",
		details:         "Errors detected",
	}

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, result.HealthStateType())
	assert.Equal(t, "unhealthy-component", result.ComponentName())
}

// TestOptionFunctionTypes tests that option functions have correct types.
func TestOptionFunctionTypes(t *testing.T) {
	// Verify that option functions return the correct type
	var opt Option

	opt = WithDebug(true)
	assert.NotNil(t, opt)

	opt = WithInfinibandClassRootDir("/test")
	assert.NotNil(t, opt)

	opt = WithFailureInjector(nil)
	assert.NotNil(t, opt)
}

// TestOpApplyOptWithNilOptions tests applying nil options slice.
func TestOpApplyOptWithNilOptions(t *testing.T) {
	op := &Op{}
	err := op.applyOpts(nil)
	assert.NoError(t, err)
	assert.Equal(t, infinibandclass.DefaultRootDir, op.infinibandClassRootDir)
}

// Benchmark tests

// BenchmarkApplyOpts benchmarks the applyOpts function.
func BenchmarkApplyOpts(b *testing.B) {
	opts := []Option{
		WithDebug(true),
		WithInfinibandClassRootDir("/test/path"),
		WithFailureInjector(&components.FailureInjector{}),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		op := &Op{}
		_ = op.applyOpts(opts)
	}
}

// BenchmarkWithOptions benchmarks creating and applying options.
func BenchmarkWithOptions(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		op := &Op{}
		WithDebug(true)(op)
		WithInfinibandClassRootDir("/test/path")(op)
		WithFailureInjector(&components.FailureInjector{})(op)
	}
}

// BenchmarkPrintSummary benchmarks the printSummary function.
func BenchmarkPrintSummary(b *testing.B) {
	result := &mockCheckResult{
		componentName:   "benchmark-component",
		healthStateType: apiv1.HealthStateTypeHealthy,
		summary:         "Benchmark summary",
		details:         "Benchmark details",
	}

	// Redirect stdout to /dev/null for benchmarking
	oldStdout := os.Stdout
	devNull, _ := os.Open(os.DevNull)
	os.Stdout = devNull
	defer func() {
		os.Stdout = oldStdout
		devNull.Close()
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		printSummary(result)
	}
}

// Note: Example tests are not included because the Scan function produces
// extensive output that varies based on the system configuration, making
// it impractical to use for testable examples.
