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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnrollCommand_MissingArgs verifies that the enroll command fails fast
// when neither --gateway nor the full bare-metal flag set is provided.
func TestEnrollCommand_MissingArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		errContains string
	}{
		{
			name:        "no flags",
			args:        []string{"fleetint", "enroll"},
			errContains: "either --gateway or --endpoint",
		},
		{
			name:        "endpoint without token and customer-id",
			args:        []string{"fleetint", "enroll", "--endpoint", "https://example.com"},
			errContains: "either --gateway or --endpoint",
		},
		{
			name:        "endpoint and token without customer-id",
			args:        []string{"fleetint", "enroll", "--endpoint", "https://example.com", "--token", "nvapi-xxx"},
			errContains: "either --gateway or --endpoint",
		},
		{
			name:        "gateway and endpoint are mutually exclusive",
			args:        []string{"fleetint", "enroll", "--gateway", "http://gw:4319", "--endpoint", "https://example.com"},
			errContains: "mutually exclusive",
		},
		{
			name:        "gateway and token are mutually exclusive",
			args:        []string{"fleetint", "enroll", "--gateway", "http://gw:4319", "--token", "nvapi-xxx"},
			errContains: "mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := App()
			app.Writer = &bytes.Buffer{}
			err := app.Run(tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}
