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

func TestValidateBackendEndpoint(t *testing.T) {
	t.Parallel()

	_, err := ValidateBackendEndpoint("https://example.com/base")
	require.NoError(t, err)

	_, err = ValidateBackendEndpoint("http://example.com/base")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "https")
}
