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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusCommandRejectsNonLocalServerURL(t *testing.T) {
	app := App()
	app.Writer = &bytes.Buffer{}

	err := app.Run([]string{"fleetint", "status", "--server-url", "http://169.254.169.254"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid server URL")
	assert.Contains(t, err.Error(), "loopback")
}

func TestInjectCommandRejectsNonLocalServerURL(t *testing.T) {
	app := App()
	app.Writer = &bytes.Buffer{}

	err := app.Run([]string{"fleetint", "inject", "--component", "cpu", "--server-url", "http://localhost@evil.example:15133"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid server URL")
	assert.Contains(t, err.Error(), "user info")
}

func TestStatusCommandRejectsServerURLWithPath(t *testing.T) {
	app := App()
	app.Writer = &bytes.Buffer{}

	err := app.Run([]string{"fleetint", "status", "--server-url", "http://localhost:15133/api"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid server URL")
	assert.Contains(t, err.Error(), "must not include a path")
}

// TestValidateOfflinePath tests the --path flag validation for offline mode.
func TestValidateOfflinePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "empty", path: "", wantErr: "must not be empty"},
		{name: "relative", path: "relative/path", wantErr: "must be an absolute path"},
		{name: "etc", path: "/etc/fleetint", wantErr: "restricted system directory"},
		{name: "usr", path: "/usr/local/share/fleetint", wantErr: "restricted system directory"},
		{name: "var", path: "/var/lib/fleetint", wantErr: "restricted system directory"},
		{name: "sys", path: "/sys/fs/cgroup", wantErr: "restricted system directory"},
		{name: "proc_subdir", path: "/proc/version", wantErr: "restricted"},
		{name: "bin_subdir", path: "/bin/subdir", wantErr: "restricted"},
		{name: "traversal_into_etc", path: "/opt/../etc/fleetint", wantErr: "restricted system directory"},
		{name: "valid_opt", path: "/opt/fleetint-data"},
		{name: "valid_data", path: "/data/gpu-health"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateOfflinePath(tc.path)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateOfflinePath_RejectsSymlink verifies that a symlinked --path is rejected.
func TestValidateOfflinePath_RejectsSymlink(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a symlink pointing to /tmp (benign target)
	symlinkPath := filepath.Join(tmpDir, "link")
	err := os.Symlink("/tmp", symlinkPath)
	require.NoError(t, err)

	err = validateOfflinePath(symlinkPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
}

// TestResolveToken_MutualExclusion verifies --token and --token-file can't be used together.
func TestResolveToken_MutualExclusion(t *testing.T) {
	app := App()
	app.Writer = &bytes.Buffer{}

	err := app.Run([]string{"fleetint", "enroll", "--endpoint", "https://example.com", "--token", "abc", "--token-file", "/dev/null"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

// TestResolveToken_FromFile verifies reading a token from a file.
func TestResolveToken_FromFile(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "token")
	err := os.WriteFile(tmpFile, []byte("  test-token-123  \n"), 0o600)
	require.NoError(t, err)

	app := App()
	app.Writer = &bytes.Buffer{}

	// The enrollment itself will fail (no server), but if we get past token
	// resolution we know the file was read and trimmed correctly. The error
	// should NOT be about token resolution.
	err = app.Run([]string{"fleetint", "enroll", "--endpoint", "https://example.com", "--token-file", tmpFile, "--force"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "token is required")
	assert.NotContains(t, err.Error(), "mutually exclusive")
}

// TestResolveToken_RequiresOne verifies that omitting both flags is an error.
func TestResolveToken_RequiresOne(t *testing.T) {
	app := App()
	app.Writer = &bytes.Buffer{}

	err := app.Run([]string{"fleetint", "enroll", "--endpoint", "https://example.com"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token is required")
}
