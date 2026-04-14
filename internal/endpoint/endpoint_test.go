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

package endpoint

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateLocalServerURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "localhost_http", raw: "http://localhost:15133"},
		{name: "loopback_ipv4", raw: "http://127.0.0.1:15133"},
		{name: "loopback_ipv6", raw: "http://[::1]:15133"},
		{name: "non_localhost_rejected", raw: "http://169.254.169.254:80", wantErr: "loopback"},
		{name: "userinfo_rejected", raw: "http://localhost@evil.example:15133", wantErr: "user info"},
		{name: "suffix_attack_rejected", raw: "http://127.0.0.1.evil.example:15133", wantErr: "loopback"},
		{name: "path_query_rejected", raw: "http://localhost:15133?x=1", wantErr: "query parameters"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ValidateLocalServerURL(tc.raw)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.Nil(t, got)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.raw, got.String())
		})
	}
}

func TestValidateLocalServerURL_UnixSocket(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		wantErr  string
		wantPath string
	}{
		{
			name:     "bare_absolute_path",
			raw:      "/run/fleetint/fleetint.sock",
			wantPath: "/run/fleetint/fleetint.sock",
		},
		{
			name:     "unix_scheme",
			raw:      "unix:///run/fleetint/fleetint.sock",
			wantPath: "/run/fleetint/fleetint.sock",
		},
		{
			name:    "unix_with_query_rejected",
			raw:     "unix:///run/fleetint/fleetint.sock?debug=1",
			wantErr: "query parameters",
		},
		{
			name:    "unix_with_fragment_rejected",
			raw:     "unix:///run/fleetint/fleetint.sock#frag",
			wantErr: "query parameters",
		},
		{
			name:    "unix_with_host_rejected",
			raw:     "unix://somehost/run/fleetint/fleetint.sock",
			wantErr: "must not include host",
		},
		{
			name:    "unix_relative_path_rejected",
			raw:     "unix://relative.sock",
			wantErr: "must not include host",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ValidateLocalServerURL(tc.raw)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, "unix", got.Scheme)
			assert.Equal(t, tc.wantPath, got.Path)
		})
	}
}

func TestNewAgentHTTPClient(t *testing.T) {
	t.Parallel()

	t.Run("tcp_client_has_timeout", func(t *testing.T) {
		u, err := ValidateLocalServerURL("http://localhost:15133")
		require.NoError(t, err)
		client := NewAgentHTTPClient(u)
		assert.NotNil(t, client)
		assert.Equal(t, 5*time.Second, client.Timeout)
	})

	t.Run("unix_client_has_timeout_and_transport", func(t *testing.T) {
		u, err := ValidateLocalServerURL("/run/fleetint/fleetint.sock")
		require.NoError(t, err)
		client := NewAgentHTTPClient(u)
		assert.NotNil(t, client)
		assert.Equal(t, 5*time.Second, client.Timeout)
		assert.NotNil(t, client.Transport, "unix client should have a custom transport")
	})
}

func TestAgentBaseURL(t *testing.T) {
	t.Parallel()

	t.Run("unix_normalizes_to_localhost", func(t *testing.T) {
		u, err := ValidateLocalServerURL("/run/fleetint/fleetint.sock")
		require.NoError(t, err)
		base := AgentBaseURL(u)
		assert.Equal(t, "http", base.Scheme)
		assert.Equal(t, "localhost", base.Host)
	})

	t.Run("tcp_passthrough", func(t *testing.T) {
		u, err := ValidateLocalServerURL("http://localhost:15133")
		require.NoError(t, err)
		base := AgentBaseURL(u)
		assert.Equal(t, "http", base.Scheme)
		assert.Equal(t, "localhost:15133", base.Host)
	})
}

func TestValidateBackendEndpoint(t *testing.T) {
	t.Parallel()

	_, err := ValidateBackendEndpoint("https://example.com/base")
	require.NoError(t, err)

	_, err = ValidateBackendEndpoint("http://example.com/base")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "https")
}
