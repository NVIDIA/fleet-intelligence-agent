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

func TestParseFromEnvRejectsComputeZoneWithoutNodeGroup(t *testing.T) {
	t.Setenv(EnvNodeGroup, "")
	t.Setenv(EnvComputeZone, "zone-a")
	t.Setenv(EnvCustomTags, "")

	_, err := ParseFromEnv()
	require.ErrorContains(t, err, ReservedKeyNodeGroup)
}

func TestParseFromEnvRejectsNodeGroupWithoutComputeZone(t *testing.T) {
	t.Setenv(EnvNodeGroup, "group-a")
	t.Setenv(EnvComputeZone, "")
	t.Setenv(EnvCustomTags, "")

	_, err := ParseFromEnv()
	require.ErrorContains(t, err, ReservedKeyComputeZone)
}

func TestParseFromEnvRejectsReservedInCustomTags(t *testing.T) {
	t.Setenv(EnvCustomTags, "nodegroup=bad")
	_, err := ParseFromEnv()
	require.ErrorContains(t, err, EnvCustomTags)
}

func TestParseCLIArgs(t *testing.T) {
	tags, err := ParseCLIArgs([]string{"--Owner=team-a", "--nodegroup=group-a", "--compute_zone=zone-b"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"nodegroup":    "group-a",
		"owner":        "team-a",
		"compute_zone": "zone-b",
	}, tags)
}

func TestParseCLIArgsRejectsComputeZoneWithoutNodeGroup(t *testing.T) {
	_, err := ParseCLIArgs([]string{"--compute_zone=zone-b"})
	require.ErrorContains(t, err, ReservedKeyNodeGroup)
}

func TestParseCLIArgsRejectsNodeGroupWithoutComputeZone(t *testing.T) {
	_, err := ParseCLIArgs([]string{"--nodegroup=group-a"})
	require.ErrorContains(t, err, ReservedKeyComputeZone)
}

func TestParseCLIArgsRequiresKeyValuePairs(t *testing.T) {
	_, err := ParseCLIArgs([]string{"--owner"})
	require.ErrorContains(t, err, "key=value")
}

func TestNormalizeAndValidateKey(t *testing.T) {
	key, err := NormalizeAndValidateKey(" Owner ")
	require.NoError(t, err)
	require.Equal(t, "owner", key)

	_, err = NormalizeAndValidateKey("")
	require.ErrorContains(t, err, "empty")

	_, err = NormalizeAndValidateKey("bad key")
	require.ErrorContains(t, err, "invalid")
}
