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

package converter

import (
	"testing"
	"time"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/eventstore"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/attestation"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/exporter/collector"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/machineinfo"
)

func TestNewOTLPConverter(t *testing.T) {
	converter := NewOTLPConverter()
	assert.NotNil(t, converter)
}

func TestOTLPConverter_Convert_EmptyData(t *testing.T) {
	data := &collector.HealthData{
		Timestamp: time.Now(),
		MachineID: "test-machine",
	}

	converter := NewOTLPConverter()
	otlpData := converter.Convert(data)

	require.NotNil(t, otlpData)
	assert.NotNil(t, otlpData.Metrics)
	assert.NotNil(t, otlpData.Logs)

	// Should have resource metrics even with empty data
	assert.Len(t, otlpData.Metrics.ResourceMetrics, 1)
	// Should have resource logs even with empty data
	assert.Len(t, otlpData.Logs.ResourceLogs, 1)
}

func TestOTLPConverter_Convert_WithMetrics(t *testing.T) {
	data := &collector.HealthData{
		Timestamp: time.Now(),
		MachineID: "test-machine",
		Metrics: metrics.Metrics{
			{
				Component:        "gpu",
				Name:             "temperature",
				UnixMilliseconds: 1699200000000,
				Value:            65.5,
				Labels:           map[string]string{"gpu_id": "0"},
			},
			{
				Component:        "cpu",
				Name:             "usage",
				UnixMilliseconds: 1699200001000,
				Value:            75.0,
				Labels:           map[string]string{"core": "0"},
			},
		},
	}

	converter := NewOTLPConverter()
	otlpData := converter.Convert(data)

	require.NotNil(t, otlpData)
	require.NotNil(t, otlpData.Metrics)
	require.Len(t, otlpData.Metrics.ResourceMetrics, 1)

	rm := otlpData.Metrics.ResourceMetrics[0]
	require.Len(t, rm.ScopeMetrics, 1)

	// Should have 2 metrics + 1 summary metric = 3 total
	metrics := rm.ScopeMetrics[0].Metrics
	assert.GreaterOrEqual(t, len(metrics), 2)

	// Verify first metric
	assert.Equal(t, "temperature", metrics[0].Name)
	assert.Contains(t, metrics[0].Description, "gpu")
}

func TestOTLPConverter_Convert_WithEvents(t *testing.T) {
	data := &collector.HealthData{
		Timestamp: time.Now(),
		MachineID: "test-machine",
		Events: eventstore.Events{
			{
				Time:      time.Date(2025, 11, 5, 12, 0, 0, 0, time.UTC),
				Component: "gpu",
				Name:      "temperature_warning",
				Type:      "warning",
				Message:   "GPU temperature high",
				ExtraInfo: map[string]string{
					"xid": "79",
				},
			},
		},
	}

	converter := NewOTLPConverter()
	otlpData := converter.Convert(data)

	require.NotNil(t, otlpData)
	require.NotNil(t, otlpData.Logs)
	require.Len(t, otlpData.Logs.ResourceLogs, 1)

	rl := otlpData.Logs.ResourceLogs[0]
	require.Len(t, rl.ScopeLogs, 1)

	// Should have at least 1 log record
	logs := rl.ScopeLogs[0].LogRecords
	assert.GreaterOrEqual(t, len(logs), 1)

	// Verify log record contains event information
	logRecord := logs[0]
	body := logRecord.Body.GetStringValue()
	assert.Contains(t, body, "gpu")
	// Body should contain either the event name or message
	assert.True(t, contains(body, "temperature_warning") || contains(body, "GPU temperature high"),
		"Log should contain event name or message")

	extraInfo := findAttribute(t, logs[0].Attributes, "extra_info").GetKvlistValue()
	require.NotNil(t, extraInfo, "event log should include structured extra_info attribute")
	assert.Equal(t, float64(79), findMapValue(t, extraInfo.Values, "xid").GetDoubleValue())
}

func TestOTLPConverter_Convert_WithEvents_EmptyExtraInfo(t *testing.T) {
	data := &collector.HealthData{
		Timestamp: time.Now(),
		MachineID: "test-machine",
		Events: eventstore.Events{
			{
				Time:      time.Date(2025, 11, 5, 12, 0, 0, 0, time.UTC),
				Component: "gpu",
				Name:      "temperature_warning",
				Type:      "warning",
				Message:   "GPU temperature high",
			},
		},
	}

	converter := NewOTLPConverter()
	otlpData := converter.Convert(data)

	require.NotNil(t, otlpData)
	require.NotNil(t, otlpData.Logs)
	require.Len(t, otlpData.Logs.ResourceLogs, 1)

	logs := otlpData.Logs.ResourceLogs[0].ScopeLogs[0].LogRecords
	require.GreaterOrEqual(t, len(logs), 1)

	extraInfo := findAttribute(t, logs[0].Attributes, "extra_info").GetKvlistValue()
	require.NotNil(t, extraInfo, "event log should always include extra_info")
	assert.Empty(t, extraInfo.Values, "event log should export empty extra_info as an empty OTLP map")
}

func TestOTLPConverter_Convert_WithEvents_StructuredExtraInfo(t *testing.T) {
	rawData := `{"time":"2026-02-20T23:22:44Z","data_source":"kmsg","xid":149}`
	data := &collector.HealthData{
		Timestamp: time.Now(),
		MachineID: "test-machine",
		Events: eventstore.Events{
			{
				Time:      time.Date(2025, 11, 5, 12, 0, 0, 0, time.UTC),
				Component: "accelerator-nvidia-error-xid",
				Name:      "error_xid",
				Type:      "Fatal",
				Message:   "XID 149 NETIR",
				ExtraInfo: map[string]string{
					"data":        rawData,
					"device_uuid": "PCI:0000:04:00",
				},
			},
		},
	}

	converter := NewOTLPConverter()
	otlpData := converter.Convert(data)

	require.NotNil(t, otlpData)
	require.NotNil(t, otlpData.Logs)
	require.Len(t, otlpData.Logs.ResourceLogs, 1)

	logs := otlpData.Logs.ResourceLogs[0].ScopeLogs[0].LogRecords
	require.GreaterOrEqual(t, len(logs), 1)

	extraInfo := findAttribute(t, logs[0].Attributes, "extra_info").GetKvlistValue()
	require.NotNil(t, extraInfo)
	assert.Equal(t, "PCI:0000:04:00", findMapValue(t, extraInfo.Values, "device_uuid").GetStringValue())

	dataValue := findMapValue(t, extraInfo.Values, "data").GetKvlistValue()
	require.NotNil(t, dataValue)
	assert.Equal(t, "2026-02-20T23:22:44Z", findMapValue(t, dataValue.Values, "time").GetStringValue())
	assert.Equal(t, "kmsg", findMapValue(t, dataValue.Values, "data_source").GetStringValue())
	assert.Equal(t, float64(149), findMapValue(t, dataValue.Values, "xid").GetDoubleValue())
}

func TestOTLPConverter_Convert_WithEvents_InvalidExtraInfoRemainsString(t *testing.T) {
	data := &collector.HealthData{
		Timestamp: time.Now(),
		MachineID: "test-machine",
		Events: eventstore.Events{
			{
				Time:      time.Date(2025, 11, 5, 12, 0, 0, 0, time.UTC),
				Component: "gpu",
				Name:      "temperature_warning",
				Type:      "warning",
				Message:   "GPU temperature high",
				ExtraInfo: map[string]string{
					"data": "{invalid",
				},
			},
		},
	}

	converter := NewOTLPConverter()
	otlpData := converter.Convert(data)

	logs := otlpData.Logs.ResourceLogs[0].ScopeLogs[0].LogRecords
	require.GreaterOrEqual(t, len(logs), 1)

	extraInfo := findAttribute(t, logs[0].Attributes, "extra_info").GetKvlistValue()
	require.NotNil(t, extraInfo)
	assert.Equal(t, "{invalid", findMapValue(t, extraInfo.Values, "data").GetStringValue())
}

func TestOTLPConverter_Convert_WithComponentData(t *testing.T) {
	rawData := `{"time":"2026-02-20T23:22:44Z","data_source":"kmsg","xid":149}`
	data := &collector.HealthData{
		Timestamp: time.Now(),
		MachineID: "test-machine",
		ComponentData: map[string]interface{}{
			"gpu": map[string]any{
				"time":           metav1.Time{Time: time.Now()},
				"component_name": "gpu",
				"health":         "healthy",
				"reason":         "All checks passed",
				"extra_info": map[string]any{
					"device_uuid": "PCI:0000:04:00",
					"data":        rawData,
				},
			},
		},
	}

	converter := NewOTLPConverter()
	otlpData := converter.Convert(data)

	require.NotNil(t, otlpData)
	require.NotNil(t, otlpData.Logs)

	rl := otlpData.Logs.ResourceLogs[0]
	logs := rl.ScopeLogs[0].LogRecords

	// Should have at least 1 log for component data
	assert.GreaterOrEqual(t, len(logs), 1)

	// Find component data log
	found := false
	for _, log := range logs {
		if contains(log.Body.GetStringValue(), "gpu") && contains(log.Body.GetStringValue(), "healthy") {
			extraInfo := findAttribute(t, log.Attributes, "extra_info").GetStringValue()
			require.NotEmpty(t, extraInfo)
			assert.Contains(t, extraInfo, `"device_uuid":"PCI:0000:04:00"`)
			assert.Contains(t, extraInfo, `"data":"{\"time\":\"2026-02-20T23:22:44Z\",\"data_source\":\"kmsg\",\"xid\":149}"`)
			found = true
			break
		}
	}
	assert.True(t, found, "Should find component data log")
}

func TestOTLPConverter_Convert_WithMachineInfo(t *testing.T) {
	data := &collector.HealthData{
		Timestamp: time.Now(),
		MachineID: "test-machine",
		MachineInfo: &machineinfo.MachineInfo{
			FleetintVersion: "0.1.5",
			OSImage:         "Ubuntu 22.04",
			KernelVersion:   "5.15.0",
			CPUInfo: &apiv1.MachineCPUInfo{
				Type:         "Intel",
				Manufacturer: "Intel",
				Architecture: "x86_64",
				LogicalCores: 8,
			},
		},
	}

	converter := NewOTLPConverter()
	otlpData := converter.Convert(data)

	require.NotNil(t, otlpData)
	require.NotNil(t, otlpData.Metrics)

	// Check resource has machine info attributes
	rm := otlpData.Metrics.ResourceMetrics[0]
	assert.NotNil(t, rm.Resource)
	assert.Greater(t, len(rm.Resource.Attributes), 0)

	// Verify machine info is embedded in resource attributes
	// The attributes may have different keys based on how machine info is flattened
	attrCount := len(rm.Resource.Attributes)
	assert.Greater(t, attrCount, 2, "Should have multiple resource attributes including machine info")

	// Check that some attributes exist (the exact key names may vary)
	attrKeys := make([]string, 0, len(rm.Resource.Attributes))
	for _, attr := range rm.Resource.Attributes {
		attrKeys = append(attrKeys, attr.Key)
	}
	// Should have at least service.name and machine.id
	hasServiceName := false
	for _, key := range attrKeys {
		if key == "service.name" {
			hasServiceName = true
			break
		}
	}
	assert.True(t, hasServiceName, "Should have service.name attribute")
}

func TestOTLPConverter_Convert_WithAttestationData(t *testing.T) {
	attestationData := &attestation.AttestationData{
		Success: true,
		SDKResponse: attestation.AttestationSDKResponse{
			Evidences: []attestation.EvidenceItem{
				{
					Arch:          "BLACKWELL",
					Certificate:   "test-cert",
					DriverVersion: "575.28",
					Evidence:      "test-evidence",
					Nonce:         "test-nonce",
					VBIOSVersion:  "96.00.AF.00.01",
					Version:       "1.0",
				},
			},
			ResultCode:    0,
			ResultMessage: "Ok",
		},
		NonceRefreshTimestamp: time.Date(2025, 11, 5, 12, 0, 0, 0, time.UTC),
	}

	data := &collector.HealthData{
		Timestamp:       time.Now(),
		MachineID:       "test-machine",
		AttestationData: attestationData,
	}

	converter := NewOTLPConverter()
	otlpData := converter.Convert(data)

	require.NotNil(t, otlpData)
	require.NotNil(t, otlpData.Logs)

	// Should NOT have attestation logs
	rl := otlpData.Logs.ResourceLogs[0]
	logs := rl.ScopeLogs[0].LogRecords
	assert.Empty(t, logs, "Should not have attestation logs")

	// Should have attestation data in resource attributes
	rm := otlpData.Metrics.ResourceMetrics[0]
	attrs := rm.Resource.Attributes
	foundAttestation := false
	for _, attr := range attrs {
		if contains(attr.Key, "attestation") {
			foundAttestation = true
			break
		}
	}
	assert.True(t, foundAttestation, "Should have attestation data in resource attributes")
}

func TestOTLPConverter_ConvertStructToOTLPAttributes(t *testing.T) {
	type TestStruct struct {
		StringField string
		IntField    int
		BoolField   bool
		TimeField   time.Time
		FloatField  float64
	}

	testData := TestStruct{
		StringField: "test-value",
		IntField:    42,
		BoolField:   true,
		TimeField:   time.Date(2025, 11, 5, 12, 0, 0, 0, time.UTC),
		FloatField:  3.14,
	}

	attrs := convertStructToOTLPAttributes(testData)

	assert.Greater(t, len(attrs), 0)

	// Find and verify attributes
	foundString := false
	foundInt := false
	foundBool := false
	foundTime := false

	for _, attr := range attrs {
		switch attr.Key {
		case "StringField":
			foundString = true
			assert.Equal(t, "test-value", attr.Value.GetStringValue())
		case "IntField":
			foundInt = true
		case "BoolField":
			foundBool = true
			assert.Equal(t, "true", attr.Value.GetStringValue())
		case "TimeField":
			foundTime = true
			assert.Contains(t, attr.Value.GetStringValue(), "2025-11-05")
		}
	}

	assert.True(t, foundString, "Should have string field")
	assert.True(t, foundInt, "Should have int field")
	assert.True(t, foundBool, "Should have bool field")
	assert.True(t, foundTime, "Should have time field")
}

func TestOTLPConverter_ConvertStructToOTLPAttributesWithPrefix(t *testing.T) {
	type NestedStruct struct {
		Name  string
		Value int
	}

	nested := NestedStruct{
		Name:  "nested",
		Value: 100,
	}

	attrs := convertStructToOTLPAttributesWithPrefix(nested, "prefix")

	assert.Greater(t, len(attrs), 0)

	// All keys should have prefix
	for _, attr := range attrs {
		assert.Contains(t, attr.Key, "prefix.")
	}
}

func TestOTLPConverter_ConvertStructToOTLPAttributes_NilStruct(t *testing.T) {
	var nilStruct *struct{}
	attrs := convertStructToOTLPAttributes(nilStruct)
	assert.Empty(t, attrs)
}

func TestOTLPConverter_ConvertStructToOTLPAttributes_NestedStruct(t *testing.T) {
	type Nested struct {
		Field1 string
		Field2 int
	}

	type Parent struct {
		Name   string
		Nested Nested
	}

	parent := Parent{
		Name: "parent",
		Nested: Nested{
			Field1: "nested-value",
			Field2: 42,
		},
	}

	attrs := convertStructToOTLPAttributes(parent)

	assert.Greater(t, len(attrs), 0)

	// Should have nested attributes with prefix
	foundNestedField := false
	for _, attr := range attrs {
		if contains(attr.Key, "Nested.Field1") {
			foundNestedField = true
			assert.Equal(t, "nested-value", attr.Value.GetStringValue())
		}
	}
	assert.True(t, foundNestedField, "Should have nested struct attributes")
}

func TestOTLPConverter_ConvertStructToOTLPAttributes_SliceField(t *testing.T) {
	type StructWithSlice struct {
		Name  string
		Items []string
	}

	data := StructWithSlice{
		Name:  "test",
		Items: []string{"item1", "item2", "item3"},
	}

	attrs := convertStructToOTLPAttributes(data)

	assert.Greater(t, len(attrs), 0)

	// Should have items as JSON string
	foundSlice := false
	for _, attr := range attrs {
		if attr.Key == "Items" {
			foundSlice = true
			// Should be JSON array
			assert.Contains(t, attr.Value.GetStringValue(), "item1")
			break
		}
	}
	assert.True(t, foundSlice, "Should have slice field as JSON")
}

func TestOTLPConverter_ConvertStructToOTLPAttributes_EmptySlice(t *testing.T) {
	type StructWithSlice struct {
		Items []string
	}

	data := StructWithSlice{
		Items: []string{},
	}

	attrs := convertStructToOTLPAttributes(data)

	// Empty slices should not be included
	for _, attr := range attrs {
		assert.NotEqual(t, "Items", attr.Key, "Empty slice should not be included")
	}
}

func TestOTLPConverter_ConvertLabelsToOTLPAttributes(t *testing.T) {
	labels := map[string]string{
		"gpu_id": "0",
		"type":   "memory",
		"status": "healthy",
	}

	converter := &otlpConverter{}
	attrs := converter.convertLabelsToOTLPAttributes(labels, nil)

	assert.Len(t, attrs, 3)

	// Verify all labels are converted
	labelMap := make(map[string]string)
	for _, attr := range attrs {
		labelMap[attr.Key] = attr.Value.GetStringValue()
	}

	assert.Equal(t, "0", labelMap["gpu_id"])
	assert.Equal(t, "memory", labelMap["type"])
	assert.Equal(t, "healthy", labelMap["status"])
}

func TestOTLPConverter_ConvertLabelsToOTLPAttributes_EmptyLabels(t *testing.T) {
	labels := map[string]string{}

	converter := &otlpConverter{}
	attrs := converter.convertLabelsToOTLPAttributes(labels, nil)

	assert.Empty(t, attrs)
}

func TestOTLPConverter_ConvertLabelsToOTLPAttributes_EnrichesGPUIndex(t *testing.T) {
	gpuUUIDToIndex := map[string]string{
		"GPU-abc-123": "0",
		"GPU-def-456": "1",
	}

	t.Run("adds gpu label when uuid present but gpu absent", func(t *testing.T) {
		labels := map[string]string{
			"uuid":           "GPU-abc-123",
			"gpud_component": "accelerator-nvidia-utilization",
		}

		converter := &otlpConverter{}
		attrs := converter.convertLabelsToOTLPAttributes(labels, gpuUUIDToIndex)

		attrMap := make(map[string]string)
		for _, attr := range attrs {
			attrMap[attr.Key] = attr.Value.GetStringValue()
		}

		assert.Equal(t, "0", attrMap["gpu"], "should enrich with gpu index from machine info")
		assert.Equal(t, "GPU-abc-123", attrMap["uuid"])
	})

	t.Run("skips enrichment when gpu label already present (DCGM)", func(t *testing.T) {
		labels := map[string]string{
			"uuid":           "GPU-abc-123",
			"gpu":            "0",
			"gpud_component": "accelerator-nvidia-dcgm-clock",
		}

		converter := &otlpConverter{}
		attrs := converter.convertLabelsToOTLPAttributes(labels, gpuUUIDToIndex)

		gpuCount := 0
		for _, attr := range attrs {
			if attr.Key == "gpu" {
				gpuCount++
			}
		}
		assert.Equal(t, 1, gpuCount, "should not duplicate gpu label for DCGM metrics")
	})

	t.Run("skips enrichment when uuid not in mapping", func(t *testing.T) {
		labels := map[string]string{
			"uuid":           "GPU-unknown-999",
			"gpud_component": "accelerator-nvidia-utilization",
		}

		converter := &otlpConverter{}
		attrs := converter.convertLabelsToOTLPAttributes(labels, gpuUUIDToIndex)

		attrMap := make(map[string]string)
		for _, attr := range attrs {
			attrMap[attr.Key] = attr.Value.GetStringValue()
		}

		_, hasGPU := attrMap["gpu"]
		assert.False(t, hasGPU, "should not add gpu label when uuid not found in mapping")
	})

	t.Run("skips enrichment when no uuid label", func(t *testing.T) {
		labels := map[string]string{
			"gpud_component": "os",
			"mount_point":    "/",
		}

		converter := &otlpConverter{}
		attrs := converter.convertLabelsToOTLPAttributes(labels, gpuUUIDToIndex)

		attrMap := make(map[string]string)
		for _, attr := range attrs {
			attrMap[attr.Key] = attr.Value.GetStringValue()
		}

		_, hasGPU := attrMap["gpu"]
		assert.False(t, hasGPU, "should not add gpu label for non-GPU metrics")
	})

	t.Run("works with nil map", func(t *testing.T) {
		labels := map[string]string{
			"uuid":           "GPU-abc-123",
			"gpud_component": "accelerator-nvidia-utilization",
		}

		converter := &otlpConverter{}
		attrs := converter.convertLabelsToOTLPAttributes(labels, nil)

		attrMap := make(map[string]string)
		for _, attr := range attrs {
			attrMap[attr.Key] = attr.Value.GetStringValue()
		}

		_, hasGPU := attrMap["gpu"]
		assert.False(t, hasGPU, "should not add gpu label when mapping is nil")
	})
}

func TestBuildGPUUUIDToIndexMap(t *testing.T) {
	t.Run("builds map from machine info", func(t *testing.T) {
		data := &collector.HealthData{
			MachineInfo: &machineinfo.MachineInfo{
				GPUInfo: &apiv1.MachineGPUInfo{
					GPUs: []apiv1.MachineGPUInstance{
						{UUID: "GPU-abc-123", GPUIndex: "0"},
						{UUID: "GPU-def-456", GPUIndex: "1"},
					},
				},
			},
		}

		m := buildGPUUUIDToIndexMap(data)
		assert.Equal(t, "0", m["GPU-abc-123"])
		assert.Equal(t, "1", m["GPU-def-456"])
		assert.Len(t, m, 2)
	})

	t.Run("returns empty map when machine info is nil", func(t *testing.T) {
		data := &collector.HealthData{}
		m := buildGPUUUIDToIndexMap(data)
		assert.Empty(t, m)
	})

	t.Run("returns empty map when GPU info is nil", func(t *testing.T) {
		data := &collector.HealthData{
			MachineInfo: &machineinfo.MachineInfo{},
		}
		m := buildGPUUUIDToIndexMap(data)
		assert.Empty(t, m)
	})

	t.Run("skips entries with empty uuid or index", func(t *testing.T) {
		data := &collector.HealthData{
			MachineInfo: &machineinfo.MachineInfo{
				GPUInfo: &apiv1.MachineGPUInfo{
					GPUs: []apiv1.MachineGPUInstance{
						{UUID: "GPU-abc-123", GPUIndex: "0"},
						{UUID: "", GPUIndex: "1"},
						{UUID: "GPU-ghi-789", GPUIndex: ""},
					},
				},
			},
		}

		m := buildGPUUUIDToIndexMap(data)
		assert.Equal(t, "0", m["GPU-abc-123"])
		assert.Len(t, m, 1)
	})
}

func TestOTLPConverter_GetAttestationEvidencesCount(t *testing.T) {
	tests := []struct {
		name          string
		data          *collector.HealthData
		expectedCount int
	}{
		{
			name: "with_evidences",
			data: &collector.HealthData{
				AttestationData: &attestation.AttestationData{
					SDKResponse: attestation.AttestationSDKResponse{
						Evidences: []attestation.EvidenceItem{
							{Arch: "BLACKWELL"},
							{Arch: "HOPPER"},
						},
					},
				},
			},
			expectedCount: 2,
		},
		{
			name: "nil_attestation",
			data: &collector.HealthData{
				AttestationData: nil,
			},
			expectedCount: 0,
		},
		{
			name: "empty_evidences",
			data: &collector.HealthData{
				AttestationData: &attestation.AttestationData{
					SDKResponse: attestation.AttestationSDKResponse{
						Evidences: []attestation.EvidenceItem{},
					},
				},
			},
			expectedCount: 0,
		},
	}

	converter := &otlpConverter{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := converter.getAttestationEvidencesCount(tt.data)
			assert.Equal(t, tt.expectedCount, count)
		})
	}
}

func TestOTLPConverter_SummaryMetric(t *testing.T) {
	data := &collector.HealthData{
		Timestamp: time.Now(),
		MachineID: "test-machine",
		Metrics: metrics.Metrics{
			{Component: "gpu", Name: "temp", Value: 65.0},
		},
		Events: eventstore.Events{
			{Component: "gpu", Name: "event1"},
		},
		ComponentData: map[string]interface{}{
			"comp1": map[string]any{"health": "healthy"},
		},
	}

	converter := NewOTLPConverter()
	otlpData := converter.Convert(data)

	rm := otlpData.Metrics.ResourceMetrics[0]
	metrics := rm.ScopeMetrics[0].Metrics

	// Find summary metric
	var summaryMetric *metricsv1.Metric
	for _, m := range metrics {
		if m.Name == "fleetint_agent_collection_summary" {
			summaryMetric = m
			break
		}
	}

	require.NotNil(t, summaryMetric, "Should have summary metric")
	assert.Contains(t, summaryMetric.Description, "collection")

	// Verify summary attributes
	gauge := summaryMetric.Data.(*metricsv1.Metric_Gauge).Gauge
	require.Len(t, gauge.DataPoints, 1)

	attrs := gauge.DataPoints[0].Attributes
	attrMap := make(map[string]int64)
	for _, attr := range attrs {
		attrMap[attr.Key] = attr.Value.GetIntValue()
	}

	assert.Equal(t, int64(1), attrMap["metrics_count"])
	assert.Equal(t, int64(1), attrMap["events_count"])
	assert.Equal(t, int64(1), attrMap["component_data_count"])
}

func TestOTLPConverter_ResourceAttributes(t *testing.T) {
	data := &collector.HealthData{
		Timestamp: time.Now(),
		MachineID: "test-machine-123",
		ComponentData: map[string]interface{}{
			"comp1": map[string]any{},
			"comp2": map[string]any{},
		},
	}

	converter := NewOTLPConverter()
	otlpData := converter.Convert(data)

	rm := otlpData.Metrics.ResourceMetrics[0]
	attrs := rm.Resource.Attributes

	// Find specific required attributes
	attrMap := make(map[string]string)
	for _, attr := range attrs {
		if attr.Value.GetStringValue() != "" {
			attrMap[attr.Key] = attr.Value.GetStringValue()
		}
	}

	assert.Equal(t, "fleet-intelligence-agent", attrMap["service.name"])
	assert.Equal(t, "test-machine-123", attrMap["machine.id"])
}

func TestOTLPConverter_Interface(t *testing.T) {
	// Verify otlpConverter implements OTLPConverter interface
	var _ OTLPConverter = (*otlpConverter)(nil)

	converter := NewOTLPConverter()
	assert.NotNil(t, converter)
}

func TestOTLPConverter_Convert_AllData(t *testing.T) {
	// Test with all data types combined
	data := &collector.HealthData{
		Timestamp: time.Now(),
		MachineID: "test-machine",
		Metrics: metrics.Metrics{
			{Component: "gpu", Name: "temp", Value: 65.0, UnixMilliseconds: time.Now().UnixMilli()},
		},
		Events: eventstore.Events{
			{Time: time.Now(), Component: "gpu", Name: "event1", Type: "info", Message: "Test event"},
		},
		ComponentData: map[string]interface{}{
			"gpu": map[string]any{
				"health": "healthy",
				"reason": "All OK",
			},
		},
		MachineInfo: &machineinfo.MachineInfo{
			FleetintVersion: "0.1.5",
		},
		AttestationData: &attestation.AttestationData{
			Success: true,
			SDKResponse: attestation.AttestationSDKResponse{
				Evidences: []attestation.EvidenceItem{
					{Arch: "BLACKWELL", VBIOSVersion: "96.00"},
				},
				ResultCode:    0,
				ResultMessage: "Ok",
			},
			NonceRefreshTimestamp: time.Now(),
		},
	}

	converter := NewOTLPConverter()
	otlpData := converter.Convert(data)

	// Verify all data types are present
	require.NotNil(t, otlpData)
	require.NotNil(t, otlpData.Metrics)
	require.NotNil(t, otlpData.Logs)

	// Verify metrics
	rm := otlpData.Metrics.ResourceMetrics[0]
	assert.Greater(t, len(rm.ScopeMetrics[0].Metrics), 0)

	// Verify logs (events + component data)
	rl := otlpData.Logs.ResourceLogs[0]
	assert.Greater(t, len(rl.ScopeLogs[0].LogRecords), 0)

	// Verify resource has attributes from machine info
	assert.Greater(t, len(rm.Resource.Attributes), 0)
}

func TestOTLPConverter_ComponentDataWithNilValues(t *testing.T) {
	data := &collector.HealthData{
		Timestamp: time.Now(),
		MachineID: "test-machine",
		ComponentData: map[string]interface{}{
			"comp1": map[string]any{
				"health":     "healthy",
				"reason":     "OK",
				"time":       nil, // nil time value
				"extra_info": nil, // nil extra info
			},
		},
	}

	converter := NewOTLPConverter()
	otlpData := converter.Convert(data)

	require.NotNil(t, otlpData)
	require.NotNil(t, otlpData.Logs)

	// Should handle nil values gracefully
	rl := otlpData.Logs.ResourceLogs[0]
	logs := rl.ScopeLogs[0].LogRecords

	// Should have at least the component log
	assert.GreaterOrEqual(t, len(logs), 1)
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func findAttribute(t *testing.T, attrs []*commonv1.KeyValue, key string) *commonv1.AnyValue {
	t.Helper()

	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Value
		}
	}

	t.Fatalf("attribute %q not found", key)
	return nil
}

func findMapValue(t *testing.T, attrs []*commonv1.KeyValue, key string) *commonv1.AnyValue {
	t.Helper()

	return findAttribute(t, attrs, key)
}
