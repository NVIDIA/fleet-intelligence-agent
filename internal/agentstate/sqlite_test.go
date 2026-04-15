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

package agentstate

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func newTestSQLiteState(t *testing.T) *sqliteState {
	t.Helper()
	stateFile := filepath.Join(t.TempDir(), "agent.state")
	return &sqliteState{
		stateFileFn: func() (string, error) {
			return stateFile, nil
		},
	}
}

func TestSQLiteStateRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := newTestSQLiteState(t)

	err := state.SetBackendBaseURL(ctx, "https://backend.example.com")
	require.NoError(t, err)
	err = state.SetJWT(ctx, "jwt-token")
	require.NoError(t, err)
	err = state.SetSAK(ctx, "sak-token")
	require.NoError(t, err)
	err = state.SetNodeID(ctx, "node-1")
	require.NoError(t, err)

	value, ok, err := state.GetBackendBaseURL(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "https://backend.example.com", value)

	value, ok, err = state.GetJWT(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "jwt-token", value)

	value, ok, err = state.GetSAK(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "sak-token", value)

	value, ok, err = state.GetNodeID(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "node-1", value)
}

func TestSQLiteStateMissingValue(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := newTestSQLiteState(t)

	err := state.SetJWT(ctx, "jwt-token")
	require.NoError(t, err)

	value, ok, err := state.GetBackendBaseURL(ctx)
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, value)
}

func TestSQLiteStateStateFileErrors(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	state := &sqliteState{
		stateFileFn: func() (string, error) {
			return "", boom
		},
	}

	_, _, err := state.GetJWT(context.Background())
	require.ErrorIs(t, err, boom)

	err = state.SetJWT(context.Background(), "jwt-token")
	require.ErrorIs(t, err, boom)
}

func TestNewSQLite(t *testing.T) {
	t.Parallel()
	require.NotNil(t, NewSQLite())
}
