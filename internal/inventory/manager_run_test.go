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

type countingSource struct {
	collectCh chan struct{}
}

func (s *countingSource) Collect(context.Context) (*Snapshot, error) {
	if s.collectCh != nil {
		s.collectCh <- struct{}{}
	}
	return &Snapshot{MachineID: "machine-1", Hostname: "host-a"}, nil
}

type blockingSource struct {
	started chan struct{}
	release chan struct{}
}

func (s *blockingSource) Collect(context.Context) (*Snapshot, error) {
	close(s.started)
	<-s.release
	return &Snapshot{MachineID: "machine-1", Hostname: "host-a"}, nil
}

func TestManagerCollectOnceErrors(t *testing.T) {
	_, err := NewManager(nil, nil, InventoryConfig{}).CollectOnce(context.Background())
	require.ErrorContains(t, err, "inventory source is required")

	_, err = NewManager(errSource{err: errors.New("boom")}, nil, InventoryConfig{}).CollectOnce(context.Background())
	require.ErrorContains(t, err, "boom")

	_, err = NewManager(nilSnapshotSource{}, nil, InventoryConfig{}).CollectOnce(context.Background())
	require.ErrorContains(t, err, "nil snapshot")
}

func TestManagerRunWithZeroInterval(t *testing.T) {
	src := &fakeSource{
		snapshots: []*Snapshot{{MachineID: "machine-1", Hostname: "host-a"}},
	}
	sink := &fakeSink{}

	err := NewManager(src, sink, InventoryConfig{}).Run(context.Background())
	require.NoError(t, err)
	require.Len(t, sink.exported, 1)
}

func TestManagerRunStopsOnContextCancel(t *testing.T) {
	src := &fakeSource{
		snapshots: []*Snapshot{{MachineID: "machine-1", Hostname: "host-a"}},
	}
	sink := &fakeSink{ready: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- NewManager(src, sink, InventoryConfig{Interval: 10 * time.Millisecond}).Run(ctx)
	}()

	select {
	case <-sink.ready:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for inventory export")
	}
	cancel()

	err := <-done
	require.ErrorIs(t, err, context.Canceled)
	require.NotEmpty(t, sink.exported)
}

func TestSleepWithContext(t *testing.T) {
	require.NoError(t, sleepWithContext(context.Background(), 0))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, sleepWithContext(ctx, 0), context.Canceled)

	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, sleepWithContext(ctx, time.Hour), context.Canceled)
}

func TestInventoryJitterHelpers(t *testing.T) {
	require.Equal(t, time.Duration(0), initialJitterCap(0))
	require.Equal(t, 15*time.Second, initialJitterCap(time.Minute))
	require.Equal(t, 30*time.Minute, initialJitterCap(4*time.Hour))

	require.Equal(t, time.Duration(0), retryJitterCap(0))
	require.Equal(t, 30*time.Second, retryJitterCap(time.Minute))
	require.Equal(t, 5*time.Minute, retryJitterCap(20*time.Minute))

	require.Equal(t, time.Duration(0), calculateJitter(0))
	jitter := calculateJitter(50 * time.Millisecond)
	require.GreaterOrEqual(t, jitter, time.Duration(0))
	require.Less(t, jitter, 50*time.Millisecond)
}

func TestManagerRunUsesRetryIntervalWithoutJitter(t *testing.T) {
	src := errSource{err: errors.New("boom")}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := NewManager(src, nil, InventoryConfig{
		Interval:      time.Hour,
		RetryInterval: 5 * time.Millisecond,
	}).Run(ctx)
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.GreaterOrEqual(t, elapsed, 15*time.Millisecond)
	require.Less(t, elapsed, 100*time.Millisecond)
}

func TestManagerRunCollectionTimeoutDoesNotOverlapStuckCollection(t *testing.T) {
	src := &blockingSource{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	mgr := NewManager(src, nil, InventoryConfig{Timeout: 10 * time.Millisecond}).(*manager)

	start := time.Now()
	_, err := mgr.collectOnceForRun(context.Background())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), time.Second)

	select {
	case <-src.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for inventory collection to start")
	}

	_, err = mgr.collectOnceForRun(context.Background())
	require.ErrorContains(t, err, "previous inventory collection is still running")

	close(src.release)
}

func TestManagerRunWaitsIntervalBeforeSecondCollect(t *testing.T) {
	src := &countingSource{collectCh: make(chan struct{}, 4)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- NewManager(src, nil, InventoryConfig{Interval: 50 * time.Millisecond}).Run(ctx)
	}()

	select {
	case <-src.collectCh:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for initial collection")
	}

	select {
	case <-src.collectCh:
		t.Fatal("second collection happened before interval elapsed")
	case <-time.After(20 * time.Millisecond):
	}

	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}
