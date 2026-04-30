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

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadEnvFileDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fleetint.env")
	require.NoError(t, os.WriteFile(path, []byte(`
# comment
FLEETINT_TEST_ENV_FILE_VALUE="from-file"
export FLEETINT_TEST_EXPORTED_VALUE='exported'
FLEETINT_TEST_EXISTING_VALUE="from-file"
`), 0o600))
	t.Setenv("FLEETINT_TEST_EXISTING_VALUE", "from-process")
	t.Cleanup(func() {
		_ = os.Unsetenv("FLEETINT_TEST_ENV_FILE_VALUE")
		_ = os.Unsetenv("FLEETINT_TEST_EXPORTED_VALUE")
	})

	require.NoError(t, LoadEnvFileDefaults(path))
	require.Equal(t, "from-file", os.Getenv("FLEETINT_TEST_ENV_FILE_VALUE"))
	require.Equal(t, "exported", os.Getenv("FLEETINT_TEST_EXPORTED_VALUE"))
	require.Equal(t, "from-process", os.Getenv("FLEETINT_TEST_EXISTING_VALUE"))
}

func TestLoadEnvFileDefaultsMissingFile(t *testing.T) {
	require.NoError(t, LoadEnvFileDefaults(filepath.Join(t.TempDir(), "missing")))
}

func TestParseEnvFileLineRejectsInvalidLine(t *testing.T) {
	_, _, _, err := parseEnvFileLine("not-an-assignment")
	require.ErrorContains(t, err, "expected KEY=VALUE")
}
