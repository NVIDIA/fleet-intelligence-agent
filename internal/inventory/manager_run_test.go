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

package inventory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type errSource struct{ err error }

func (s errSource) Collect(context.Context) (*Snapshot, error) { return nil, s.err }

type nilSnapshotSource struct{}

func (nilSnapshotSource) Collect(context.Context) (*Snapshot, error) { return nil, nil }

func TestManagerCollectOnceErrors(t *testing.T) {
	_, err := NewManager(nil, nil, 0).CollectOnce(context.Background())
	require.ErrorContains(t, err, "inventory source is required")

	_, err = NewManager(errSource{err: errors.New("boom")}, nil, 0).CollectOnce(context.Background())
	require.ErrorContains(t, err, "boom")

	_, err = NewManager(nilSnapshotSource{}, nil, 0).CollectOnce(context.Background())
	require.ErrorContains(t, err, "nil snapshot")
}

func TestManagerRunWithZeroInterval(t *testing.T) {
	src := &fakeSource{
		snapshots: []*Snapshot{{NodeID: "node-1", MachineID: "machine-1", Hostname: "host-a"}},
	}
	sink := &fakeSink{}

	err := NewManager(src, sink, 0).Run(context.Background())
	require.NoError(t, err)
	require.Len(t, sink.exported, 1)
}

func TestManagerRunStopsOnContextCancel(t *testing.T) {
	src := &fakeSource{
		snapshots: []*Snapshot{{NodeID: "node-1", MachineID: "machine-1", Hostname: "host-a"}},
	}
	sink := &fakeSink{}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- NewManager(src, sink, 10*time.Millisecond).Run(ctx)
	}()

	time.Sleep(25 * time.Millisecond)
	cancel()

	err := <-done
	require.ErrorIs(t, err, context.Canceled)
	require.NotEmpty(t, sink.exported)
}
