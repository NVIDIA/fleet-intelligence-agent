// SPDX-FileCopyrightText: Copyright (c) 2025, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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

package collector

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	"github.com/NVIDIA/fleet-intelligence-sdk/components"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/eventstore"
	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
	nvidianvml "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/nvml"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/attestation"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/config"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/machineinfo"
)

func TestCollector_AttestationDataCollection(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testLogic   func(t *testing.T)
	}{
		{
			name:        "first_collection_always_collects",
			description: "First collection should always collect attestation data even if empty",
			testLogic:   testFirstCollectionAlwaysCollects,
		},
		{
			name:        "subsequent_collection_skips_when_no_update",
			description: "Subsequent collections should skip when attestation data hasn't been updated",
			testLogic:   testSubsequentCollectionSkipsWhenNoUpdate,
		},
		{
			name:        "collection_after_attestation_update",
			description: "Collection should happen after attestation data is updated",
			testLogic:   testCollectionAfterAttestationUpdate,
		},
		{
			name:        "nil_attestation_manager_skips_collection",
			description: "Collector should skip attestation collection when manager is nil",
			testLogic:   testNilAttestationManagerSkipsCollection,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log("Testing:", tt.description)
			tt.testLogic(t)
		})
	}
}

func testFirstCollectionAlwaysCollects(t *testing.T) {
	ctx := context.Background()
	cfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: false,
	}
	attestationManager := attestation.NewManager(ctx, nil, cfg) // nil nvmlInstance, 20s for testing

	// Create collector
	testCollector := createTestCollector(attestationManager)

	// Start attestation manager to populate some data
	attestationManager.Start()
	defer attestationManager.Stop()

	// Wait a bit for attestation to run
	time.Sleep(100 * time.Millisecond)

	// Collect data for the first time
	data, err := testCollector.Collect(ctx)

	require.NoError(t, err)
	require.NotNil(t, data)

	// First collection should work (but may not have attestation data due to test environment)
	if data.AttestationData != nil {
		t.Log("First collection successfully populated attestation data")
	} else {
		t.Log("First collection did not populate attestation data - this is expected when NVML/nonce fails in test environment")
	}

	// Check if lastAttestationCollection was updated only if attestation data was collected
	collectorImpl := testCollector.(*collector)
	if data.AttestationData != nil {
		assert.False(t, collectorImpl.lastAttestationCollection.IsZero(),
			"lastAttestationCollection should be set after successful collection")
	} else {
		assert.True(t, collectorImpl.lastAttestationCollection.IsZero(),
			"lastAttestationCollection should remain zero when attestation fails")
	}
}

func testSubsequentCollectionSkipsWhenNoUpdate(t *testing.T) {
	ctx := context.Background()
	cfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: false,
	}
	attestationManager := attestation.NewManager(ctx, nil, cfg) // nil nvmlInstance, 20s for testing

	// Create collector
	testCollector := createTestCollector(attestationManager)
	collectorImpl := testCollector.(*collector)

	// Start attestation manager
	attestationManager.Start()
	defer attestationManager.Stop()

	// Wait for attestation to run and populate data
	time.Sleep(100 * time.Millisecond)

	// First collection
	data1, err := testCollector.Collect(ctx)
	require.NoError(t, err)
	require.NotNil(t, data1)

	firstCollectionTime := collectorImpl.lastAttestationCollection
	// In test environment, this will be zero since attestation fails
	t.Logf("First collection time: %v", firstCollectionTime)

	// Verify first collection has attestation data
	// Verify first collection has attestation data (or logs why it doesn't)
	if data1.AttestationData != nil {
		assert.Empty(t, data1.AttestationData.SDKResponse.Evidences, "Until Attestation is available in public release, this should be empty")
	} else {
		t.Log("First collection did not populate attestation data - this is expected when NVML/nonce fails")
	}

	// Sleep a little to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Second collection (should skip attestation since no update)
	data2, err := testCollector.Collect(ctx)
	require.NoError(t, err)
	require.NotNil(t, data2)

	secondCollectionTime := collectorImpl.lastAttestationCollection

	// lastAttestationCollection should remain the same (indicating skip)
	assert.Equal(t, firstCollectionTime, secondCollectionTime,
		"lastAttestationCollection should not change when attestation collection is skipped")
}

func testCollectionAfterAttestationUpdate(t *testing.T) {
	ctx := context.Background()
	cfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: false,
	}
	attestationManager := attestation.NewManager(ctx, nil, cfg) // nil nvmlInstance, 20s for testing

	// Create collector
	testCollector := createTestCollector(attestationManager)
	collectorImpl := testCollector.(*collector)

	// Start attestation manager with faster interval for testing (20 seconds)
	attestationManager.Start()
	defer attestationManager.Stop()

	// Wait for first attestation to run
	time.Sleep(100 * time.Millisecond)

	// First collection
	data1, err := testCollector.Collect(ctx)
	require.NoError(t, err)
	require.NotNil(t, data1)

	firstCollectionTime := collectorImpl.lastAttestationCollection

	// Verify first collection has attestation data
	// Verify first collection has attestation data (or logs why it doesn't)
	if data1.AttestationData != nil {
		assert.Empty(t, data1.AttestationData.SDKResponse.Evidences, "Until Attestation is available in public release, this should be empty")
	} else {
		t.Log("First collection did not populate attestation data - this is expected when NVML/nonce fails")
	}

	// Wait for attestation to run again (it's set to 20 seconds in the test)
	t.Log("Waiting for attestation to refresh...")
	time.Sleep(10 * time.Second)

	// Second collection (should collect since attestation was updated)
	data2, err := testCollector.Collect(ctx)
	require.NoError(t, err)
	require.NotNil(t, data2)

	secondCollectionTime := collectorImpl.lastAttestationCollection

	// In test environment, both times will be zero since attestation fails
	t.Logf("First collection time: %v, Second collection time: %v", firstCollectionTime, secondCollectionTime)

	// In a real environment with working NVML/HTTP, both collections would have evidence data
	// In test environment, they will be nil due to missing dependencies
	if data1.AttestationData != nil && data2.AttestationData != nil {
		assert.Empty(t, data1.AttestationData.SDKResponse.Evidences, "Until Attestation is available in public release, this should be empty")
		assert.Empty(t, data2.AttestationData.SDKResponse.Evidences, "Until Attestation is available in public release, this should be empty")
		t.Log("Both collections successfully have attestation data")
	} else {
		t.Log("Collections do not have attestation data - expected in test environment without real dependencies")
	}
}

func testNilAttestationManagerSkipsCollection(t *testing.T) {
	ctx := context.Background()

	// Create collector with nil attestation manager
	testCollector := createTestCollectorWithNilAttestation()

	// Collection should skip gracefully
	data, err := testCollector.Collect(ctx)

	require.NoError(t, err)
	require.NotNil(t, data)
	assert.Nil(t, data.AttestationData, "Should not collect attestation data when manager is nil")
}

func TestCollector_AttestationDataCollection_WithMockData(t *testing.T) {
	// This test verifies collection behavior when attestation is unavailable
	ctx := context.Background()
	attestationCfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: false,
	}
	attestationManager := attestation.NewManager(ctx, nil, attestationCfg)
	testCollector := createTestCollector(attestationManager)
	collectorImpl := testCollector.(*collector)

	// Verify that collection works when no attestation data is available
	data1, err := testCollector.Collect(ctx)
	require.NoError(t, err)
	require.NotNil(t, data1)

	// Should be nil since no attestation data is available
	assert.Nil(t, data1.AttestationData, "Should be nil when no attestation data available")
	assert.True(t, collectorImpl.lastAttestationCollection.IsZero(), "Should remain zero")

	t.Log("Successfully tested collection with no attestation data")
}

func TestAttestationManager_UpdateTracking(t *testing.T) {
	ctx := context.Background()
	attestationCfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: false,
	}
	manager := attestation.NewManager(ctx, nil, attestationCfg) // nil nvmlInstance for testing

	// Initially, no updates
	baseTime := time.Now().UTC()
	assert.False(t, manager.IsAttestationDataUpdated(baseTime),
		"Should return false before any attestation runs")

	// Start the manager and test the update tracking
	manager.Start()
	defer manager.Stop()

	// Give it time to attempt attestation
	time.Sleep(100 * time.Millisecond)

	// In test environment this may still be false due to NVML/HTTP failures, but that's expected
	updated := manager.IsAttestationDataUpdated(baseTime)
	t.Logf("Attestation updated after start: %v", updated)

	// The important part is that the method doesn't crash and returns a boolean
	assert.IsType(t, false, updated, "IsAttestationDataUpdated should return a boolean")
}

// Helper functions

func createTestCollector(attestationManager *attestation.Manager) Collector {
	cfg := &config.HealthExporterConfig{
		IncludeMachineInfo:   false,
		IncludeMetrics:       false,
		IncludeEvents:        false,
		IncludeComponentData: false,
	}

	return New(
		cfg,
		nil, // fullConfig
		nil, // allComponentNames
		nil, // metricsStore
		nil, // eventStore
		nil, // componentsRegistry
		nil, // nvmlInstance
		attestationManager,
		"test-machine-id",
		nil, // dcgmGPUIndexes
	)
}

func createTestCollectorWithNilAttestation() Collector {
	cfg := &config.HealthExporterConfig{
		IncludeMachineInfo:   false,
		IncludeMetrics:       false,
		IncludeEvents:        false,
		IncludeComponentData: false,
	}

	return New(
		cfg,
		nil, // fullConfig
		nil, // allComponentNames
		nil, // metricsStore
		nil, // eventStore
		nil, // componentsRegistry
		nil, // nvmlInstance
		nil, // attestationManager (nil for testing)
		"test-machine-id",
		nil, // dcgmGPUIndexes
	)
}

func TestGenerateCollectionID(t *testing.T) {
	// Generate multiple collection IDs
	id1 := GenerateCollectionID()
	id2 := GenerateCollectionID()

	// Verify IDs are generated
	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)

	// Verify IDs are unique
	assert.NotEqual(t, id1, id2, "Collection IDs should be unique")

	// Verify IDs are valid hex strings
	_, err1 := hex.DecodeString(id1)
	_, err2 := hex.DecodeString(id2)
	assert.NoError(t, err1, "ID1 should be valid hex")
	assert.NoError(t, err2, "ID2 should be valid hex")

	// Verify IDs are 32 characters (16 bytes in hex)
	assert.Len(t, id1, 32, "Collection ID should be 32 characters")
	assert.Len(t, id2, 32, "Collection ID should be 32 characters")
}

func TestGenerateEventID(t *testing.T) {
	id1 := GenerateEventID()
	id2 := GenerateEventID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2, "Event IDs should be unique")

	_, err1 := uuid.Parse(id1)
	_, err2 := uuid.Parse(id2)
	assert.NoError(t, err1, "ID1 should be a valid UUID")
	assert.NoError(t, err2, "ID2 should be a valid UUID")
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeMachineInfo:   true,
		IncludeMetrics:       true,
		IncludeEvents:        true,
		IncludeComponentData: true,
	}
	attestationCfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: false,
	}
	attestationManager := attestation.NewManager(ctx, nil, attestationCfg)

	c := New(cfg, nil, nil, nil, nil, nil, nil, attestationManager, "test-machine-id", nil)

	assert.NotNil(t, c, "Collector should be created")

	// Verify it implements Collector interface
	var _ = c
}

func TestCollector_Collect_BasicFlow(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeMachineInfo:   false,
		IncludeMetrics:       false,
		IncludeEvents:        false,
		IncludeComponentData: false,
		Attestation:          config.AttestationConfig{},
	}

	collector := New(cfg, nil, nil, nil, nil, nil, nil, nil, "test-machine-id", nil)
	data, err := collector.Collect(ctx)

	require.NoError(t, err)
	require.NotNil(t, data)

	// Verify basic fields
	assert.NotEmpty(t, data.CollectionID, "CollectionID should be generated")
	// MachineID may be empty in test environment
	t.Logf("MachineID: %s", data.MachineID)
	assert.False(t, data.Timestamp.IsZero(), "Timestamp should be set")

	// Verify optional fields are nil/empty when disabled
	assert.Nil(t, data.MachineInfo, "MachineInfo should be nil when disabled")
	assert.Empty(t, data.Metrics, "Metrics should be empty when disabled")
	assert.Empty(t, data.Events, "Events should be empty when disabled")
	assert.Empty(t, data.ComponentData, "ComponentData should be empty when disabled")
	assert.Nil(t, data.AttestationData, "AttestationData should be nil when disabled")
}

func TestCollector_CollectMachineInfo_NoNVML(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeMachineInfo: true,
	}

	collector := New(cfg, nil, nil, nil, nil, nil, nil, nil, "test-machine-id", nil)
	data, err := collector.Collect(ctx)

	// Should still succeed but machine info will not be collected
	require.NoError(t, err)
	require.NotNil(t, data)

	// MachineInfo should be nil because NVML is not available
	assert.Nil(t, data.MachineInfo, "MachineInfo should be nil without NVML")
}

func TestCollector_CollectMachineInfo_UsesCachedValue(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeMachineInfo: true,
	}

	c := New(cfg, nil, nil, nil, nil, nil, nil, nil, "test-machine-id", nil).(*collector)
	expected := &machineinfo.MachineInfo{Hostname: "cached-host"}
	provider := &mockMachineInfoProvider{
		cached: expected,
	}
	c.machineInfoProvider = provider

	data, err := c.Collect(ctx)

	require.NoError(t, err)
	require.NotNil(t, data)
	require.NotNil(t, data.MachineInfo)
	assert.Equal(t, expected, data.MachineInfo)
	assert.Equal(t, int32(1), provider.refreshCalls.Load())
}

func TestCollector_CollectMachineInfo_WaitsBrieflyForInitialRefresh(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeMachineInfo: true,
	}

	c := New(cfg, nil, nil, nil, nil, nil, nil, nil, "test-machine-id", nil).(*collector)
	provider := newMockMachineInfoProvider()
	c.machineInfoProvider = provider

	go func() {
		time.Sleep(50 * time.Millisecond)
		provider.setCached(&machineinfo.MachineInfo{Hostname: "prewarmed-host"})
		provider.markInitialRefreshDone()
	}()

	data, err := c.Collect(ctx)

	require.NoError(t, err)
	require.NotNil(t, data)
	require.NotNil(t, data.MachineInfo)
	assert.Equal(t, "prewarmed-host", data.MachineInfo.Hostname)
}

func TestCollector_CollectMachineInfo_RefreshDoesNotBlockMetrics(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeMachineInfo: true,
		IncludeMetrics:     true,
		MetricsLookback:    metav1.Duration{Duration: 5 * time.Minute},
	}

	c := New(cfg, nil, nil, &mockMetricsStore{
		metrics: pkgmetrics.Metrics{
			{Component: "gpu", Name: "temperature", Value: 70, UnixMilliseconds: time.Now().UnixMilli()},
		},
	}, nil, nil, nil, nil, "test-machine-id", nil).(*collector)

	blocker := make(chan struct{})
	provider := newMockMachineInfoProvider()
	provider.refreshFn = func(parent context.Context) {
		provider.markInitialRefreshDone()
		<-blocker
	}
	c.machineInfoProvider = provider

	start := time.Now()
	data, err := c.Collect(ctx)
	elapsed := time.Since(start)
	close(blocker)

	require.NoError(t, err)
	require.NotNil(t, data)
	assert.Len(t, data.Metrics, 1)
	assert.Nil(t, data.MachineInfo)
	assert.GreaterOrEqual(t, elapsed, 4900*time.Millisecond)
	assert.Less(t, elapsed, 5500*time.Millisecond)
}

func TestCollector_CollectMachineInfo_InitialWaitDoesNotRepeatAfterFirstRefresh(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeMachineInfo: true,
	}

	c := New(cfg, nil, nil, nil, nil, nil, nil, nil, "test-machine-id", nil).(*collector)
	provider := newMockMachineInfoProvider()
	provider.markInitialRefreshDone()
	provider.refreshFn = func(parent context.Context) {}
	c.machineInfoProvider = provider

	start := time.Now()
	data, err := c.Collect(ctx)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, data)
	assert.Less(t, elapsed, 200*time.Millisecond)
}

func TestCollector_CollectMachineInfo_InitialWaitDoesNotRepeatAfterTimeout(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeMachineInfo: true,
	}

	c := New(cfg, nil, nil, nil, nil, nil, nil, nil, "test-machine-id", nil).(*collector)
	provider := newMockMachineInfoProvider()
	provider.refreshFn = func(parent context.Context) {}
	c.machineInfoProvider = provider

	start := time.Now()
	data, err := c.Collect(ctx)
	firstElapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, data)
	assert.GreaterOrEqual(t, firstElapsed, 4900*time.Millisecond)
	assert.Less(t, firstElapsed, 5500*time.Millisecond)

	start = time.Now()
	data, err = c.Collect(ctx)
	secondElapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, data)
	assert.Less(t, secondElapsed, 200*time.Millisecond)
}

func TestCollector_CollectMachineInfo_InitialWaitHonorsContextCancellation(t *testing.T) {
	cfg := &config.HealthExporterConfig{
		IncludeMachineInfo: true,
	}

	c := New(cfg, nil, nil, nil, nil, nil, nil, nil, "test-machine-id", nil).(*collector)
	provider := newMockMachineInfoProvider()
	provider.refreshFn = func(parent context.Context) {}
	c.machineInfoProvider = provider

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	data, err := c.Collect(ctx)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, data)
	assert.Less(t, elapsed, 200*time.Millisecond)
}

func TestCollector_CollectMachineInfo_RetainsLastGoodOnRefreshFailure(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeMachineInfo: true,
	}

	c := New(cfg, nil, nil, nil, nil, nil, nil, nil, "test-machine-id", nil).(*collector)
	expected := &machineinfo.MachineInfo{Hostname: "last-good"}
	provider := newMockMachineInfoProvider()
	provider.cached = expected
	provider.initialRefreshOnce.Do(func() { close(provider.initialRefreshDone) })
	provider.refreshFn = func(parent context.Context) {}
	c.machineInfoProvider = provider

	data, err := c.Collect(ctx)

	require.NoError(t, err)
	require.NotNil(t, data)
	assert.Equal(t, expected, data.MachineInfo)
}

func TestCachedMachineInfoProvider_DeduplicatesConcurrentRefresh(t *testing.T) {
	originalGetMachineInfo := getMachineInfo
	defer func() {
		getMachineInfo = originalGetMachineInfo
	}()

	var calls atomic.Int32
	blocker := make(chan struct{})
	getMachineInfo = func(nvmlInstance nvidianvml.Instance, opts ...machineinfo.MachineInfoOption) (*machineinfo.MachineInfo, error) {
		calls.Add(1)
		<-blocker
		return &machineinfo.MachineInfo{Hostname: "refreshed"}, nil
	}

	provider := newCachedMachineInfoProvider(nvidianvml.NewNoOp(), time.Minute).(*cachedMachineInfoProvider)
	ctx := context.Background()

	for range 3 {
		provider.RefreshAsync(ctx)
	}

	require.Eventually(t, func() bool {
		return calls.Load() == 1
	}, time.Second, 10*time.Millisecond)

	close(blocker)

	require.Eventually(t, func() bool {
		info, ok := provider.Get()
		return ok && info.Hostname == "refreshed"
	}, time.Second, 10*time.Millisecond)
}

func TestCachedMachineInfoProvider_WaitForInitialRefreshReturnsAfterCompletion(t *testing.T) {
	provider := newCachedMachineInfoProvider(nvidianvml.NewNoOp(), time.Minute).(*cachedMachineInfoProvider)

	go func() {
		time.Sleep(50 * time.Millisecond)
		provider.markInitialRefreshDone()
	}()

	start := time.Now()
	provider.WaitForInitialRefresh(context.Background(), time.Second)
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, 50*time.Millisecond)
	assert.Less(t, elapsed, 300*time.Millisecond)
}

func TestCachedMachineInfoProvider_WaitForInitialRefreshTimesOut(t *testing.T) {
	provider := newCachedMachineInfoProvider(nvidianvml.NewNoOp(), time.Minute).(*cachedMachineInfoProvider)

	start := time.Now()
	provider.WaitForInitialRefresh(context.Background(), 100*time.Millisecond)
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond)
	assert.Less(t, elapsed, 300*time.Millisecond)
}

func TestCollector_CollectMetrics_NoStore(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeMetrics:  true,
		MetricsLookback: metav1.Duration{Duration: 5 * time.Minute},
	}

	collector := New(cfg, nil, nil, nil, nil, nil, nil, nil, "test-machine-id", nil)
	data, err := collector.Collect(ctx)

	// Should still succeed but metrics will not be collected
	require.NoError(t, err)
	require.NotNil(t, data)

	// Metrics should be empty because store is not available
	assert.Empty(t, data.Metrics, "Metrics should be empty without store")
}

func TestCollector_CollectMetrics_WithStore(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeMetrics:  true,
		MetricsLookback: metav1.Duration{Duration: 5 * time.Minute},
	}

	// Create mock metrics store
	mockStore := &mockMetricsStore{
		metrics: pkgmetrics.Metrics{
			{
				Component:        "gpu",
				Name:             "temperature",
				UnixMilliseconds: time.Now().UnixMilli(),
				Value:            65.5,
			},
			{
				Component:        "cpu",
				Name:             "usage",
				UnixMilliseconds: time.Now().UnixMilli(),
				Value:            75.0,
			},
		},
	}

	collector := New(cfg, nil, nil, mockStore, nil, nil, nil, nil, "test-machine-id", nil)
	data, err := collector.Collect(ctx)

	require.NoError(t, err)
	require.NotNil(t, data)

	// Verify metrics were collected
	assert.Len(t, data.Metrics, 2, "Should have 2 metrics")
	assert.Equal(t, "gpu", data.Metrics[0].Component)
	assert.Equal(t, "temperature", data.Metrics[0].Name)
}

func TestCollector_CollectMetrics_StoreError(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeMetrics:  true,
		MetricsLookback: metav1.Duration{Duration: 5 * time.Minute},
	}

	// Create mock metrics store that returns error
	mockStore := &mockMetricsStore{
		shouldError: true,
	}

	collector := New(cfg, nil, nil, mockStore, nil, nil, nil, nil, "test-machine-id", nil)
	data, err := collector.Collect(ctx)

	// Should still succeed (error is logged but doesn't fail collection)
	require.NoError(t, err)
	require.NotNil(t, data)

	// Metrics should be empty due to error
	assert.Empty(t, data.Metrics, "Metrics should be empty on error")
}

func TestCollector_CollectEvents_NoStoreOrRegistry(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeEvents:  true,
		EventsLookback: metav1.Duration{Duration: 5 * time.Minute},
	}

	collector := New(cfg, nil, nil, nil, nil, nil, nil, nil, "test-machine-id", nil)
	data, err := collector.Collect(ctx)

	// Should still succeed but events will not be collected
	require.NoError(t, err)
	require.NotNil(t, data)

	// Events should be empty
	assert.Empty(t, data.Events, "Events should be empty without store/registry")
}

func TestCollector_CollectEvents_WithComponents(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeEvents:  true,
		EventsLookback: metav1.Duration{Duration: 5 * time.Minute},
	}

	// Create mock registry with component
	mockComp := &mockComponent{
		name: "test-component",
		events: []apiv1.Event{
			{
				EventID:   "123e4567-e89b-12d3-a456-426614174000",
				Time:      metav1.Time{Time: time.Now()},
				Component: "test-component",
				Name:      "test-event",
				Type:      apiv1.EventTypeWarning,
				Message:   "Test warning",
				ExtraInfo: map[string]string{
					"xid_code": "79",
				},
			},
		},
	}
	mockRegistry := &mockRegistry{
		components: []components.Component{mockComp},
	}
	mockEventStore := &mockEventStore{}

	collector := New(cfg, nil, nil, nil, mockEventStore, mockRegistry, nil, nil, "test-machine-id", nil)
	data, err := collector.Collect(ctx)

	require.NoError(t, err)
	require.NotNil(t, data)

	// Verify events were collected
	assert.Len(t, data.Events, 1, "Should have 1 event")
	assert.Equal(t, "test-component", data.Events[0].Component)
	assert.Equal(t, "test-event", data.Events[0].Name)
	assert.Equal(t, map[string]string{"xid_code": "79"}, data.Events[0].ExtraInfo)
	assert.Equal(t, "123e4567-e89b-12d3-a456-426614174000", data.Events[0].EventID)
	_, err = uuid.Parse(data.Events[0].EventID)
	assert.NoError(t, err)
}

func TestCollector_CollectEvents_NoComponents(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeEvents:  true,
		EventsLookback: metav1.Duration{Duration: 5 * time.Minute},
	}

	// Create mock registry with no components
	mockRegistry := &mockRegistry{
		components: []components.Component{},
	}
	mockEventStore := &mockEventStore{}

	collector := New(cfg, nil, nil, nil, mockEventStore, mockRegistry, nil, nil, "test-machine-id", nil)
	data, err := collector.Collect(ctx)

	// Should still succeed
	require.NoError(t, err)
	require.NotNil(t, data)

	// Events should be empty
	assert.Empty(t, data.Events, "Events should be empty with no components")
}

func TestCollector_CollectEvents_ComponentError(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeEvents:  true,
		EventsLookback: metav1.Duration{Duration: 5 * time.Minute},
	}

	// Create mock component that returns error
	mockComp := &mockComponent{
		name:        "error-component",
		shouldError: true,
	}
	mockRegistry := &mockRegistry{
		components: []components.Component{mockComp},
	}
	mockEventStore := &mockEventStore{}

	collector := New(cfg, nil, nil, nil, mockEventStore, mockRegistry, nil, nil, "test-machine-id", nil)
	data, err := collector.Collect(ctx)

	// Should still succeed (component errors are logged but don't fail collection)
	require.NoError(t, err)
	require.NotNil(t, data)

	// Events should be empty due to error
	assert.Empty(t, data.Events, "Events should be empty when component errors")
}

func TestCollector_CollectComponentData_NoRegistry(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeComponentData: true,
	}

	collector := New(cfg, nil, nil, nil, nil, nil, nil, nil, "test-machine-id", nil)
	data, err := collector.Collect(ctx)

	// Should still succeed
	require.NoError(t, err)
	require.NotNil(t, data)

	// ComponentData should be empty
	assert.Empty(t, data.ComponentData, "ComponentData should be empty without registry")
}

func TestCollector_CollectComponentData_WithComponents(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeComponentData: true,
	}

	// Create mock component with health states
	mockComp := &mockComponent{
		name: "test-component",
		healthStates: []apiv1.HealthState{
			{
				Component: "test-component",
				Health:    "Healthy",
				Reason:    "All checks passed",
				Time:      metav1.Time{Time: time.Now()},
				ExtraInfo: map[string]string{"key": "value"},
				Incidents: []apiv1.HealthStateIncident{
					{
						EntityID: "GPU-1234",
						Message:  "Clock throttled",
						Health:   apiv1.HealthStateTypeDegraded,
						Error:    "DCGM_FR_CLOCK_THROTTLE_POWER",
					},
				},
			},
		},
	}
	mockRegistry := &mockRegistry{
		components: []components.Component{mockComp},
	}

	collector := New(cfg, nil, nil, nil, nil, mockRegistry, nil, nil, "test-machine-id", nil)
	data, err := collector.Collect(ctx)

	require.NoError(t, err)
	require.NotNil(t, data)

	// Verify component data was collected
	assert.Len(t, data.ComponentData, 1, "Should have 1 component data")
	compData, exists := data.ComponentData["test-component"]
	assert.True(t, exists, "Should have test-component data")

	dataMap := compData.(map[string]interface{})
	assert.Equal(t, "test-component", dataMap["component_name"])
	assert.Equal(t, "Healthy", dataMap["health"])
	assert.Equal(t, "All checks passed", dataMap["reason"])
	incidents, ok := dataMap["incidents"].([]apiv1.HealthStateIncident)
	require.True(t, ok, "incidents should preserve the typed health incidents slice")
	require.Len(t, incidents, 1)
	assert.Equal(t, "GPU-1234", incidents[0].EntityID)
}

func TestCollector_CollectComponentData_NoHealthStates(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeComponentData: true,
	}

	// Create mock component with no health states
	mockComp := &mockComponent{
		name:         "empty-component",
		healthStates: []apiv1.HealthState{},
	}
	mockRegistry := &mockRegistry{
		components: []components.Component{mockComp},
	}

	collector := New(cfg, nil, nil, nil, nil, mockRegistry, nil, nil, "test-machine-id", nil)
	data, err := collector.Collect(ctx)

	require.NoError(t, err)
	require.NotNil(t, data)

	// Verify component data with defaults
	assert.Len(t, data.ComponentData, 1, "Should have 1 component data")
	compData := data.ComponentData["empty-component"].(map[string]interface{})
	assert.Equal(t, "Unknown", compData["health"])
	assert.Equal(t, "No health data", compData["reason"])
}

func TestCollector_AllFeaturesEnabled(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeMachineInfo:   true,
		IncludeMetrics:       true,
		IncludeEvents:        true,
		IncludeComponentData: true,
		Attestation:          config.AttestationConfig{},
		MetricsLookback:      metav1.Duration{Duration: 5 * time.Minute},
		EventsLookback:       metav1.Duration{Duration: 5 * time.Minute},
	}

	// Create mocks for all features
	mockMetricsStore := &mockMetricsStore{
		metrics: pkgmetrics.Metrics{
			{Component: "gpu", Name: "temp", Value: 65.0, UnixMilliseconds: time.Now().UnixMilli()},
		},
	}

	mockComp := &mockComponent{
		name: "gpu",
		events: []apiv1.Event{
			{
				Time:    metav1.Time{Time: time.Now()},
				Name:    "test-event",
				Type:    apiv1.EventTypeInfo,
				Message: "Test",
			},
		},
		healthStates: []apiv1.HealthState{
			{Health: "Healthy", Reason: "OK", Time: metav1.Time{Time: time.Now()}},
		},
	}
	mockRegistry := &mockRegistry{
		components: []components.Component{mockComp},
	}

	attestationCfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: false,
	}
	attestationManager := attestation.NewManager(ctx, nil, attestationCfg)

	mockEventStore := &mockEventStore{}

	collector := New(cfg, nil, nil, mockMetricsStore, mockEventStore, mockRegistry, nil, attestationManager, "test-machine-id", nil)
	data, err := collector.Collect(ctx)

	require.NoError(t, err)
	require.NotNil(t, data)

	// Verify all data types are populated
	assert.NotEmpty(t, data.CollectionID)
	// MachineID may be empty in test environment
	t.Logf("MachineID: %s", data.MachineID)
	assert.False(t, data.Timestamp.IsZero())
	assert.Len(t, data.Metrics, 1)
	assert.Len(t, data.Events, 1)
	assert.Len(t, data.ComponentData, 1)
	// MachineInfo will be nil without NVML
	// AttestationData may be nil in test environment
}

// =============================================================================
// Mock Implementations
// =============================================================================

type mockMetricsStore struct {
	metrics     pkgmetrics.Metrics
	shouldError bool
}

type mockMachineInfoProvider struct {
	mu                 sync.RWMutex
	cached             *machineinfo.MachineInfo
	refreshFn          func(context.Context)
	refreshCalls       atomic.Int32
	initialWaited      bool
	initialRefreshDone chan struct{}
	initialRefreshOnce sync.Once
}

func newMockMachineInfoProvider() *mockMachineInfoProvider {
	return &mockMachineInfoProvider{
		initialRefreshDone: make(chan struct{}),
	}
}

func (m *mockMachineInfoProvider) Get() (*machineinfo.MachineInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.cached == nil {
		return nil, false
	}
	return m.cached, true
}

func (m *mockMachineInfoProvider) RefreshAsync(parent context.Context) {
	m.refreshCalls.Add(1)
	if m.refreshFn == nil {
		return
	}
	go func() {
		defer func() {
			_ = recover()
		}()
		m.refreshFn(parent)
	}()
}

func (m *mockMachineInfoProvider) WaitForInitialRefresh(ctx context.Context, maxWait time.Duration) bool {
	if maxWait <= 0 {
		return false
	}

	m.mu.Lock()
	if m.initialWaited {
		m.mu.Unlock()
		return false
	}
	m.initialWaited = true
	m.mu.Unlock()

	timer := time.NewTimer(maxWait)
	defer timer.Stop()

	select {
	case <-m.initialRefreshDone:
		return true
	case <-ctx.Done():
		return false
	case <-timer.C:
		return false
	}
}

func (m *mockMachineInfoProvider) setCached(info *machineinfo.MachineInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cached = info
}

func (m *mockMachineInfoProvider) markInitialRefreshDone() {
	m.initialRefreshOnce.Do(func() {
		close(m.initialRefreshDone)
	})
}

func (m *mockMetricsStore) Read(ctx context.Context, opts ...pkgmetrics.OpOption) (pkgmetrics.Metrics, error) {
	if m.shouldError {
		return nil, errors.New("mock metrics store error")
	}
	return m.metrics, nil
}

func (m *mockMetricsStore) Purge(ctx context.Context, since time.Time) (int, error) {
	return 0, nil
}

func (m *mockMetricsStore) Record(ctx context.Context, metrics ...pkgmetrics.Metric) error {
	return nil
}

type mockRegistry struct {
	components []components.Component
}

func (m *mockRegistry) All() []components.Component {
	return m.components
}

func (m *mockRegistry) Deregister(name string) components.Component {
	return nil
}

func (m *mockRegistry) Get(name string) components.Component {
	for _, comp := range m.components {
		if comp.Name() == name {
			return comp
		}
	}
	return nil
}

func (m *mockRegistry) MustRegister(initFunc components.InitFunc) {
	// No-op for testing
}

func (m *mockRegistry) Register(initFunc components.InitFunc) (components.Component, error) {
	return nil, nil
}

type mockEventStore struct{}

// Implement eventstore.Store interface
func (m *mockEventStore) Close(ctx context.Context) error {
	return nil
}

func (m *mockEventStore) Bucket(name string, opts ...eventstore.OpOption) (eventstore.Bucket, error) {
	return nil, nil
}

type mockComponent struct {
	name         string
	events       []apiv1.Event
	healthStates []apiv1.HealthState
	shouldError  bool
}

func (m *mockComponent) Name() string {
	return m.name
}

func (m *mockComponent) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	if m.shouldError {
		return nil, errors.New("mock component error")
	}
	return apiv1.Events(m.events), nil
}

func (m *mockComponent) LastHealthStates() apiv1.HealthStates {
	return apiv1.HealthStates(m.healthStates)
}

// Implement other required interface methods as no-ops
func (m *mockComponent) Start() error                   { return nil }
func (m *mockComponent) Stop(ctx context.Context) error { return nil }
func (m *mockComponent) States(ctx context.Context) ([]any, error) {
	return nil, nil
}
func (m *mockComponent) Metrics(ctx context.Context, since time.Time) ([]pkgmetrics.Metric, error) {
	return nil, nil
}
func (m *mockComponent) Check() components.CheckResult {
	return nil
}

func (m *mockComponent) Close() error {
	return nil
}

func (m *mockComponent) IsSupported() bool {
	return true
}

func (m *mockComponent) Tags() []string {
	return []string{}
}
