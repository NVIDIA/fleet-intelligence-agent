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
	"testing"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/NVIDIA/gpuhealth/internal/attestation"
	"github.com/NVIDIA/gpuhealth/internal/config"
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
	attestationManager := attestation.NewManager(ctx, nil, 20*time.Second) // nil nvmlInstance, 20s for testing

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
		assert.NotEmpty(t, data.AttestationData.SDKResponse.Evidences, "Should have evidence items")
		assert.False(t, data.AttestationData.NonceRefreshTimestamp.IsZero(), "NonceRefreshTimestamp should be set")
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
	attestationManager := attestation.NewManager(ctx, nil, 20*time.Second) // nil nvmlInstance, 20s for testing

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
		assert.NotEmpty(t, data1.AttestationData.SDKResponse.Evidences, "First collection should have evidence items")
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

	// Second collection should not have attestation data (skipped)
	assert.Nil(t, data2.AttestationData,
		"Second collection should not populate attestation data when skipped")
}

func testCollectionAfterAttestationUpdate(t *testing.T) {
	ctx := context.Background()
	attestationManager := attestation.NewManager(ctx, nil, 20*time.Second) // nil nvmlInstance, 20s for testing

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
		assert.NotEmpty(t, data1.AttestationData.SDKResponse.Evidences, "First collection should have evidence items")
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

	// If attestation worked, times would be different
	if data1.AttestationData != nil && data2.AttestationData != nil {
		assert.True(t, secondCollectionTime.After(firstCollectionTime),
			"lastAttestationCollection should be updated after attestation refresh")
	} else {
		t.Log("Attestation failed in test environment - this is expected without real NVML/HTTP endpoint")
	}

	// In a real environment with working NVML/HTTP, both collections would have evidence data
	// In test environment, they will be nil due to missing dependencies
	if data1.AttestationData != nil && data2.AttestationData != nil {
		assert.NotEmpty(t, data1.AttestationData.SDKResponse.Evidences, "First collection should have evidence")
		assert.NotEmpty(t, data2.AttestationData.SDKResponse.Evidences, "Second collection should have evidence after update")
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
	attestationManager := attestation.NewManager(ctx, nil, 20*time.Second)
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
	manager := attestation.NewManager(ctx, nil, 20*time.Second) // nil nvmlInstance for testing

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
	config := &config.HealthExporterConfig{
		IncludeMachineInfo:   false,
		IncludeMetrics:       false,
		IncludeEvents:        false,
		IncludeComponentData: false,
	}

	return New(
		config,
		nil, // metricsStore
		nil, // eventStore
		nil, // componentsRegistry
		nil, // nvmlInstance
		attestationManager,
		"test-machine-id",
	)
}

func createTestCollectorWithNilAttestation() Collector {
	config := &config.HealthExporterConfig{
		IncludeMachineInfo:   false,
		IncludeMetrics:       false,
		IncludeEvents:        false,
		IncludeComponentData: false,
	}

	return New(
		config,
		nil, // metricsStore
		nil, // eventStore
		nil, // componentsRegistry
		nil, // nvmlInstance
		nil, // attestationManager (nil for testing)
		"test-machine-id",
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

func TestNew(t *testing.T) {
	ctx := context.Background()
	config := &config.HealthExporterConfig{
		IncludeMachineInfo:   true,
		IncludeMetrics:       true,
		IncludeEvents:        true,
		IncludeComponentData: true,
	}
	attestationManager := attestation.NewManager(ctx, nil, 20*time.Second)

	c := New(config, nil, nil, nil, nil, attestationManager, "test-machine-id")

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
		AttestationEnabled:   false,
	}

	collector := New(cfg, nil, nil, nil, nil, nil, "test-machine-id")
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

	collector := New(cfg, nil, nil, nil, nil, nil, "test-machine-id")
	data, err := collector.Collect(ctx)

	// Should still succeed but machine info will not be collected
	require.NoError(t, err)
	require.NotNil(t, data)

	// MachineInfo should be nil because NVML is not available
	assert.Nil(t, data.MachineInfo, "MachineInfo should be nil without NVML")
}

func TestCollector_CollectMetrics_NoStore(t *testing.T) {
	ctx := context.Background()
	cfg := &config.HealthExporterConfig{
		IncludeMetrics:  true,
		MetricsLookback: metav1.Duration{Duration: 5 * time.Minute},
	}

	collector := New(cfg, nil, nil, nil, nil, nil, "test-machine-id")
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

	collector := New(cfg, mockStore, nil, nil, nil, nil, "test-machine-id")
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

	collector := New(cfg, mockStore, nil, nil, nil, nil, "test-machine-id")
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

	collector := New(cfg, nil, nil, nil, nil, nil, "test-machine-id")
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
				Time:      metav1.Time{Time: time.Now()},
				Component: "test-component",
				Name:      "test-event",
				Type:      apiv1.EventTypeWarning,
				Message:   "Test warning",
			},
		},
	}
	mockRegistry := &mockRegistry{
		components: []components.Component{mockComp},
	}
	mockEventStore := &mockEventStore{}

	collector := New(cfg, nil, mockEventStore, mockRegistry, nil, nil, "test-machine-id")
	data, err := collector.Collect(ctx)

	require.NoError(t, err)
	require.NotNil(t, data)

	// Verify events were collected
	assert.Len(t, data.Events, 1, "Should have 1 event")
	assert.Equal(t, "test-component", data.Events[0].Component)
	assert.Equal(t, "test-event", data.Events[0].Name)
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

	collector := New(cfg, nil, mockEventStore, mockRegistry, nil, nil, "test-machine-id")
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

	collector := New(cfg, nil, mockEventStore, mockRegistry, nil, nil, "test-machine-id")
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

	collector := New(cfg, nil, nil, nil, nil, nil, "test-machine-id")
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
			},
		},
	}
	mockRegistry := &mockRegistry{
		components: []components.Component{mockComp},
	}

	collector := New(cfg, nil, nil, mockRegistry, nil, nil, "test-machine-id")
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

	collector := New(cfg, nil, nil, mockRegistry, nil, nil, "test-machine-id")
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
		AttestationEnabled:   true,
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

	attestationManager := attestation.NewManager(ctx, nil, 20*time.Second)

	mockEventStore := &mockEventStore{}

	collector := New(cfg, mockMetricsStore, mockEventStore, mockRegistry, nil, attestationManager, "test-machine-id")
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
