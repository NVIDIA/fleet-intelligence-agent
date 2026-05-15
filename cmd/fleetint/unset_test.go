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
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/agentstate"
	agenttag "github.com/NVIDIA/fleet-intelligence-agent/internal/tag"
)

func TestUnsetCommandPersistsAndSyncs(t *testing.T) {
	useMissingFleetintEnvFile(t)

	originalUpsert := upsertTagsAfterTagUpdate
	originalStateFactory := newTagCommandState
	state := &inMemoryTagState{
		tags: map[string]string{
			agenttag.ReservedKeyNodeGroup:   "group-a",
			agenttag.ReservedKeyComputeZone: "zone-a",
			"owner":                         "team-a",
			"cost_center":                   "ml",
		},
	}
	t.Cleanup(func() {
		upsertTagsAfterTagUpdate = originalUpsert
		newTagCommandState = originalStateFactory
	})
	newTagCommandState = func() agentstate.State {
		return state
	}

	var gotPatch map[string]string
	upsertTagsAfterTagUpdate = func(_ context.Context, patch map[string]string) error {
		gotPatch = patch
		return nil
	}

	app := App()
	app.Writer = &bytes.Buffer{}
	err := app.Run([]string{"fleetint", "unset", "--nodegroup", "--tag", "owner"})
	require.NoError(t, err)

	tags, ok, err := state.GetTags(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, map[string]string{
		agenttag.ReservedKeyComputeZone: "zone-a",
		"cost_center":                   "ml",
	}, tags)
	require.Equal(t, map[string]string{
		agenttag.ReservedKeyNodeGroup: "",
		"owner":                       "",
	}, gotPatch)
}

func TestUnsetCommandRequiresTarget(t *testing.T) {
	app := App()
	app.Writer = &bytes.Buffer{}
	err := app.Run([]string{"fleetint", "unset"})
	require.ErrorContains(t, err, "at least one target must be provided")
}

func TestUnsetCommandRejectsReservedTagFlag(t *testing.T) {
	app := App()
	app.Writer = &bytes.Buffer{}
	err := app.Run([]string{"fleetint", "unset", "--tag", "nodegroup"})
	require.ErrorContains(t, err, "must use --nodegroup or --compute-zone")
}

func TestUnsetCommandRejectsComputeZoneWithoutNodeGroup(t *testing.T) {
	app := App()
	app.Writer = &bytes.Buffer{}
	err := app.Run([]string{"fleetint", "unset", "--compute-zone"})
	require.ErrorContains(t, err, agenttag.ReservedKeyNodeGroup)
}
