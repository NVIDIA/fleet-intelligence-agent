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
