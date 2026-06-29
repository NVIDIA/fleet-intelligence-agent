// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvlink

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	nvmldevice "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/nvml/device"
)

type fakeNVLinkSourceDevice struct {
	uuid            string
	fabricState     nvmldevice.FabricState
	fabricErr       error
	nvlinkStates    map[int]nvml.EnableState
	nvlinkReturns   map[int]nvml.Return
	getFieldsReturn nvml.Return
	linkCountReturn nvml.Return
	linkCountType   nvml.ValueType
	linkCount       uint32
	speedReturn     nvml.Return
	speedType       nvml.ValueType
	speedMBytesPerS uint32
	getFieldsCalls  int
}

func (f *fakeNVLinkSourceDevice) UUID() string {
	return f.uuid
}

func (f *fakeNVLinkSourceDevice) GetFabricState() (nvmldevice.FabricState, error) {
	if f.fabricErr != nil {
		return nvmldevice.FabricState{}, f.fabricErr
	}
	return f.fabricState, nil
}

func (f *fakeNVLinkSourceDevice) GetNvLinkState(link int) (nvml.EnableState, nvml.Return) {
	if ret, ok := f.nvlinkReturns[link]; ok {
		return f.nvlinkStates[link], ret
	}
	return nvml.FEATURE_ENABLED, nvml.ERROR_INVALID_ARGUMENT
}

func (f *fakeNVLinkSourceDevice) GetFieldValues(values []nvml.FieldValue) nvml.Return {
	f.getFieldsCalls++
	if f.getFieldsReturn != 0 && f.getFieldsReturn != nvml.SUCCESS {
		return f.getFieldsReturn
	}
	for i := range values {
		switch values[i].FieldId {
		case nvml.FI_DEV_NVLINK_LINK_COUNT:
			values[i] = fieldValue(values[i].FieldId, f.linkCount, f.linkCountType, f.linkCountReturn)
		case nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON:
			values[i] = fieldValue(values[i].FieldId, f.speedMBytesPerS, f.speedType, f.speedReturn)
		}
	}
	return nvml.SUCCESS
}

func fieldValue(fieldID uint32, value uint32, valueType nvml.ValueType, ret nvml.Return) nvml.FieldValue {
	if valueType == 0 {
		valueType = nvml.VALUE_TYPE_UNSIGNED_INT
	}
	var raw [8]byte
	binary.LittleEndian.PutUint32(raw[:4], value)
	return nvml.FieldValue{
		FieldId:    fieldID,
		ValueType:  uint32(valueType),
		NvmlReturn: uint32(ret),
		Value:      raw,
	}
}

func TestCollectNVLinkSourceMetricsRecordsExpectedSignals(t *testing.T) {
	dev := &fakeNVLinkSourceDevice{
		uuid:        "GPU-a",
		fabricState: nvmldevice.FabricState{HealthMask: 0x55},
		nvlinkStates: map[int]nvml.EnableState{
			0: nvml.FEATURE_ENABLED,
			1: nvml.FEATURE_DISABLED,
			2: nvml.FEATURE_DISABLED,
		},
		nvlinkReturns: map[int]nvml.Return{
			0: nvml.SUCCESS,
			1: nvml.SUCCESS,
			2: nvml.SUCCESS,
			3: nvml.ERROR_INVALID_ARGUMENT,
		},
		linkCount:       18,
		speedMBytesPerS: 50000,
	}
	devices := map[string]*fakeNVLinkSourceDevice{"GPU-a": dev}

	metrics := collectNVLinkSourceMetrics(devices, map[string]string{"GPU-a": "3"})

	require.Len(t, metrics, 4)
	requireSourceMetric(t, metrics, MetricNVLinkFabricHealthMask, 85, "GPU-a", "3")
	requireSourceMetric(t, metrics, MetricNVLinkInactiveCount, 2, "GPU-a", "3")
	requireSourceMetric(t, metrics, MetricNVLinkLinkCount, 18, "GPU-a", "3")
	requireSourceMetric(t, metrics, MetricNVLinkSpeedMBytesPerSec, 50000, "GPU-a", "3")
	require.Equal(t, 1, dev.getFieldsCalls)
}

func TestCollectNVLinkSourceMetricsMapsUnsupportedAndErrorsToSentinels(t *testing.T) {
	devices := map[string]*fakeNVLinkSourceDevice{
		"GPU-b": {
			uuid:            "GPU-b",
			fabricErr:       errors.New("fabric state telemetry not supported"),
			nvlinkReturns:   map[int]nvml.Return{0: nvml.ERROR_NOT_SUPPORTED},
			getFieldsReturn: nvml.ERROR_UNKNOWN,
		},
	}

	metrics := collectNVLinkSourceMetrics(devices, nil)

	requireSourceMetric(t, metrics, MetricNVLinkFabricHealthMask, SentinelUnsupported, "GPU-b", "")
	requireSourceMetric(t, metrics, MetricNVLinkInactiveCount, SentinelUnsupported, "GPU-b", "")
	requireSourceMetric(t, metrics, MetricNVLinkLinkCount, SentinelCollectionError, "GPU-b", "")
	requireSourceMetric(t, metrics, MetricNVLinkSpeedMBytesPerSec, SentinelCollectionError, "GPU-b", "")
	require.Equal(t, 2, countCollectionErrors(metrics))
}

func TestCollectNVLinkSourceMetricsMapsPerFieldFailures(t *testing.T) {
	devices := map[string]*fakeNVLinkSourceDevice{
		"GPU-c": {
			uuid:        "GPU-c",
			fabricState: nvmldevice.FabricState{HealthMask: 0},
			nvlinkReturns: map[int]nvml.Return{
				0: nvml.SUCCESS,
			},
			linkCountReturn: nvml.ERROR_NOT_SUPPORTED,
			speedReturn:     nvml.SUCCESS,
			speedType:       nvml.VALUE_TYPE_UNSIGNED_LONG_LONG,
			speedMBytesPerS: 50000,
		},
	}

	metrics := collectNVLinkSourceMetrics(devices, nil)

	requireSourceMetric(t, metrics, MetricNVLinkLinkCount, SentinelUnsupported, "GPU-c", "")
	requireSourceMetric(t, metrics, MetricNVLinkSpeedMBytesPerSec, SentinelCollectionError, "GPU-c", "")
	require.Equal(t, 1, countCollectionErrors(metrics))
}

func requireSourceMetric(t *testing.T, metrics []nvlinkSourceMetric, name string, value float64, uuid string, gpu string) {
	t.Helper()
	for _, metric := range metrics {
		if metric.name != name {
			continue
		}
		require.Equal(t, value, metric.value)
		require.Equal(t, uuid, metric.uuid)
		require.Equal(t, gpu, metric.gpu)
		return
	}
	t.Fatalf("metric %s not found", name)
}
