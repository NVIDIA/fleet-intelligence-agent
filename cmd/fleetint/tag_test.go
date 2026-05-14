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
	"time"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/agentstate"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/config"
	agenttag "github.com/NVIDIA/fleet-intelligence-agent/internal/tag"
)

type inMemoryTagState struct {
	tags map[string]string
}

func (s *inMemoryTagState) GetBackendBaseURL(context.Context) (string, bool, error) {
	return "", false, nil
}
func (s *inMemoryTagState) SetBackendBaseURL(context.Context, string) error   { return nil }
func (s *inMemoryTagState) GetJWT(context.Context) (string, bool, error)      { return "", false, nil }
func (s *inMemoryTagState) SetJWT(context.Context, string) error              { return nil }
func (s *inMemoryTagState) GetSAK(context.Context) (string, bool, error)      { return "", false, nil }
func (s *inMemoryTagState) SetSAK(context.Context, string) error              { return nil }
func (s *inMemoryTagState) GetNodeUUID(context.Context) (string, bool, error) { return "", false, nil }
func (s *inMemoryTagState) SetNodeUUID(context.Context, string) error         { return nil }
func (s *inMemoryTagState) GetEnrollmentTime(context.Context) (time.Time, bool, error) {
	return time.Time{}, false, nil
}
func (s *inMemoryTagState) SetEnrollmentTime(context.Context, time.Time) error { return nil }
func (s *inMemoryTagState) GetTags(context.Context) (map[string]string, bool, error) {
	if s.tags == nil {
		return nil, false, nil
	}
	out := make(map[string]string, len(s.tags))
	for key, value := range s.tags {
		out[key] = value
	}
	return out, true, nil
}
func (s *inMemoryTagState) SetTags(_ context.Context, value map[string]string) error {
	s.tags = make(map[string]string, len(value))
	for key, val := range value {
		s.tags[key] = val
	}
	return nil
}

func TestTagCommandPersistsAndSyncs(t *testing.T) {
	useMissingFleetintEnvFile(t)

	originalSync := syncInventoryAfterTagUpdate
	originalConfigLoader := defaultConfigForTagCommand
	originalStateFactory := newTagCommandState
	state := &inMemoryTagState{}
	t.Cleanup(func() {
		syncInventoryAfterTagUpdate = originalSync
		defaultConfigForTagCommand = originalConfigLoader
		newTagCommandState = originalStateFactory
	})
	newTagCommandState = func() agentstate.State {
		return state
	}

	defaultConfigForTagCommand = func(context.Context) (*config.Config, error) {
		return &config.Config{}, nil
	}
	syncCalled := false
	syncInventoryAfterTagUpdate = func(context.Context, *config.Config) error {
		syncCalled = true
		return nil
	}

	app := App()
	app.Writer = &bytes.Buffer{}
	err := app.Run([]string{"fleetint", "tag", "--owner=ml-platform", "--compute_zone=us-west-2a"})
	require.NoError(t, err)
	require.True(t, syncCalled)

	tags, ok, err := state.GetTags(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, map[string]string{
		"owner":                         "ml-platform",
		agenttag.ReservedKeyNodeGroup:   agenttag.DefaultReservedValueUnassigned,
		agenttag.ReservedKeyComputeZone: "us-west-2a",
	}, tags)
}

func TestTagCommandOverwritesExistingKeys(t *testing.T) {
	useMissingFleetintEnvFile(t)

	originalSync := syncInventoryAfterTagUpdate
	originalConfigLoader := defaultConfigForTagCommand
	originalStateFactory := newTagCommandState
	state := &inMemoryTagState{
		tags: map[string]string{
			agenttag.ReservedKeyNodeGroup:   "group-a",
			agenttag.ReservedKeyComputeZone: "zone-a",
			"owner":                         "team-a",
		},
	}
	t.Cleanup(func() {
		syncInventoryAfterTagUpdate = originalSync
		defaultConfigForTagCommand = originalConfigLoader
		newTagCommandState = originalStateFactory
	})
	newTagCommandState = func() agentstate.State {
		return state
	}

	defaultConfigForTagCommand = func(context.Context) (*config.Config, error) {
		return &config.Config{}, nil
	}
	syncInventoryAfterTagUpdate = func(context.Context, *config.Config) error { return nil }

	app := App()
	app.Writer = &bytes.Buffer{}
	err := app.Run([]string{"fleetint", "tag", "--owner=team-b"})
	require.NoError(t, err)

	tags, ok, err := state.GetTags(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, map[string]string{
		agenttag.ReservedKeyNodeGroup:   "group-a",
		agenttag.ReservedKeyComputeZone: "zone-a",
		"owner":                         "team-b",
	}, tags)
}

func TestTagCommandRequiresAssignments(t *testing.T) {
	app := App()
	app.Writer = &bytes.Buffer{}
	err := app.Run([]string{"fleetint", "tag"})
	require.ErrorContains(t, err, "at least one tag assignment is required")
}
