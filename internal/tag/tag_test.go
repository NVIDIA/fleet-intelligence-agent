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

package tag

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFromEnv(t *testing.T) {
	t.Setenv(EnvNodeGroup, "group-a")
	t.Setenv(EnvComputeZone, "zone-a")
	t.Setenv(EnvCustomTags, "owner=ml-platform,cost_center=cc-10")

	tags, err := ParseFromEnv()
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		ReservedKeyNodeGroup:   "group-a",
		ReservedKeyComputeZone: "zone-a",
		"owner":                "ml-platform",
		"cost_center":          "cc-10",
	}, tags)
}

func TestParseFromEnvRejectsReservedInCustomTags(t *testing.T) {
	t.Setenv(EnvCustomTags, "nodegroup=bad")
	_, err := ParseFromEnv()
	require.ErrorContains(t, err, EnvCustomTags)
}

func TestParseCLIArgs(t *testing.T) {
	tags, err := ParseCLIArgs([]string{"--Owner=team-a", "--compute_zone=zone-b"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"owner":        "team-a",
		"compute_zone": "zone-b",
	}, tags)
}

func TestEnsureReservedDefaults(t *testing.T) {
	tags := EnsureReservedDefaults(map[string]string{"owner": "team-a"})
	require.Equal(t, "team-a", tags["owner"])
	require.Equal(t, DefaultReservedValueUnassigned, tags[ReservedKeyNodeGroup])
	require.Equal(t, DefaultReservedValueUnassigned, tags[ReservedKeyComputeZone])
}
