// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package nvlink records NVIDIA NVLink source signals used by backend health-check evaluation.
package nvlink

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	"github.com/NVIDIA/fleet-intelligence-sdk/components"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	nvidiadcgm "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/dcgm"
	nvidianvml "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/nvml"
	nvmldevice "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/nvml/device"
	nvmlerrors "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia/errors"
)

const Name = "accelerator-nvidia-nvlink"

const (
	defaultHealthCheckInterval = time.Minute

	SentinelUnsupported     = -2
	SentinelCollectionError = -3
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	healthCheckInterval time.Duration

	nvmlInstance nvidianvml.Instance
	dcgmInstance nvidiadcgm.Instance

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	healthCheckInterval := defaultHealthCheckInterval
	if gpudInstance.HealthCheckInterval > 0 {
		healthCheckInterval = gpudInstance.HealthCheckInterval
	}

	c := &component{
		ctx:                 cctx,
		cancel:              ccancel,
		healthCheckInterval: healthCheckInterval,
		nvmlInstance:        gpudInstance.NVMLInstance,
		dcgmInstance:        gpudInstance.DCGMInstance,
	}
	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Tags() []string {
	return []string{
		"accelerator",
		"gpu",
		"nvidia",
		"nvlink",
		Name,
	}
}

func (c *component) IsSupported() bool {
	if c.nvmlInstance == nil {
		return false
	}
	return c.nvmlInstance.NVMLExists() && c.nvmlInstance.ProductName() != ""
}

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(c.healthCheckInterval)
		defer ticker.Stop()

		for {
			_ = c.Check()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.lastMu.RLock()
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()
	return lastCheckResult.HealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")
	c.cancel()
	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu NVLink source metrics")

	cr := &checkResult{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	if c.nvmlInstance == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML instance is nil"
		return cr
	}
	if !c.nvmlInstance.NVMLExists() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML library is not loaded"
		return cr
	}
	if c.nvmlInstance.ProductName() == "" {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML is loaded but GPU is not detected (missing product name)"
		return cr
	}

	metrics := collectNVLinkSourceMetrics(c.nvmlInstance.Devices(), c.dcgmGPUIndexes())
	recordNVLinkSourceMetrics(metrics)

	cr.metricCount = len(metrics)
	cr.collectionErrorCount = countCollectionErrors(metrics)
	if cr.collectionErrorCount > 0 {
		cr.health = apiv1.HealthStateTypeDegraded
		cr.reason = fmt.Sprintf("recorded %d NVLink source metrics with %d collection error(s)", cr.metricCount, cr.collectionErrorCount)
		return cr
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = fmt.Sprintf("recorded %d NVLink source metrics", cr.metricCount)
	return cr
}

func (c *component) dcgmGPUIndexes() map[string]string {
	indexes := make(map[string]string)
	if c == nil || c.dcgmInstance == nil || !c.dcgmInstance.DCGMExists() {
		return indexes
	}
	for _, device := range c.dcgmInstance.GetDevices() {
		if device.UUID != "" {
			indexes[device.UUID] = fmt.Sprintf("%d", device.ID)
		}
	}
	return indexes
}

type nvlinkSourceDevice interface {
	UUID() string
	GetFabricState() (nvmldevice.FabricState, error)
	GetNvLinkState(link int) (nvml.EnableState, nvml.Return)
	GetFieldValues(values []nvml.FieldValue) nvml.Return
}

type nvlinkSourceMetric struct {
	name  string
	value float64
	uuid  string
	gpu   string
}

func collectNVLinkSourceMetrics[D nvlinkSourceDevice](devices map[string]D, gpuIndexes map[string]string) []nvlinkSourceMetric {
	metrics := make([]nvlinkSourceMetric, 0, len(devices)*4)
	for key, dev := range devices {
		gpuUUID := dev.UUID()
		if gpuUUID == "" {
			gpuUUID = key
		}
		gpuIndex := ""
		if gpuIndexes != nil {
			gpuIndex = gpuIndexes[gpuUUID]
		}
		linkCount, speed := collectLinkCountAndSpeed(dev)

		metrics = append(metrics,
			nvlinkSourceMetric{name: MetricNVLinkFabricHealthMask, value: collectFabricHealthMask(dev), uuid: gpuUUID, gpu: gpuIndex},
			nvlinkSourceMetric{name: MetricNVLinkInactiveCount, value: collectInactiveCount(dev), uuid: gpuUUID, gpu: gpuIndex},
			nvlinkSourceMetric{name: MetricNVLinkLinkCount, value: linkCount, uuid: gpuUUID, gpu: gpuIndex},
			nvlinkSourceMetric{name: MetricNVLinkSpeedMBytesPerSec, value: speed, uuid: gpuUUID, gpu: gpuIndex},
		)
	}
	return metrics
}

func countCollectionErrors(metrics []nvlinkSourceMetric) int {
	var count int
	for _, metric := range metrics {
		if int64(metric.value) == SentinelCollectionError {
			count++
		}
	}
	return count
}

func collectFabricHealthMask(dev nvlinkSourceDevice) float64 {
	state, err := dev.GetFabricState()
	if err != nil {
		if isUnsupportedError(err) {
			return SentinelUnsupported
		}
		return SentinelCollectionError
	}
	return float64(state.HealthMask)
}

func collectInactiveCount(dev nvlinkSourceDevice) float64 {
	inactive := 0
	sawSuccess := false
	sawUnsupported := false

	for link := 0; link < nvml.NVLINK_MAX_LINKS; link++ {
		state, ret := dev.GetNvLinkState(link)
		switch {
		case ret == nvml.SUCCESS:
			sawSuccess = true
			if state == nvml.FEATURE_DISABLED {
				inactive++
			}
		case ret == nvml.ERROR_INVALID_ARGUMENT:
			continue
		case nvmlerrors.IsNotSupportError(ret):
			sawUnsupported = true
			continue
		default:
			return SentinelCollectionError
		}
	}

	if !sawSuccess && sawUnsupported {
		return SentinelUnsupported
	}
	return float64(inactive)
}

func collectLinkCountAndSpeed(dev nvlinkSourceDevice) (float64, float64) {
	values := []nvml.FieldValue{
		{FieldId: nvml.FI_DEV_NVLINK_LINK_COUNT},
		{FieldId: nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON},
	}
	ret := dev.GetFieldValues(values)
	if ret != nvml.SUCCESS {
		sentinel := float64(SentinelCollectionError)
		if nvmlerrors.IsNotSupportError(ret) {
			sentinel = SentinelUnsupported
		}
		return sentinel, sentinel
	}

	return decodeUnsignedIntField(values[0]), decodeUnsignedIntField(values[1])
}

func decodeUnsignedIntField(value nvml.FieldValue) float64 {
	ret := nvml.Return(value.NvmlReturn)
	if ret != nvml.SUCCESS {
		if nvmlerrors.IsNotSupportError(ret) {
			return SentinelUnsupported
		}
		return SentinelCollectionError
	}
	if nvml.ValueType(value.ValueType) != nvml.VALUE_TYPE_UNSIGNED_INT {
		return SentinelCollectionError
	}
	return float64(binary.LittleEndian.Uint32(value.Value[:4]))
}

func isUnsupportedError(err error) bool {
	return strings.Contains(strings.ToLower(fmt.Sprint(err)), "not supported")
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	ts time.Time

	health apiv1.HealthStateType
	reason string

	metricCount          int
	collectionErrorCount int
}

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}
	return cr.reason
}

func (cr *checkResult) Summary() string {
	if cr == nil {
		return ""
	}
	return cr.reason
}

func (cr *checkResult) HealthStateType() apiv1.HealthStateType {
	if cr == nil {
		return ""
	}
	return cr.health
}

func (cr *checkResult) HealthStates() apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{
			{
				Time:      metav1.NewTime(time.Now().UTC()),
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	return apiv1.HealthStates{{
		Time:      metav1.NewTime(cr.ts),
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Health:    cr.health,
	}}
}
