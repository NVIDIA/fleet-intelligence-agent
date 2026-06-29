// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvlink

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

const (
	MetricNVLinkFabricHealthMask  = "hc_nvlink_fabric_health_mask_result"
	MetricNVLinkInactiveCount     = "hc_nvlink_inactive_count_result"
	MetricNVLinkLinkCount         = "hc_nvlink_link_count_result"
	MetricNVLinkSpeedMBytesPerSec = "hc_nvlink_speed_mbytes_per_sec_result"
)

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricNVLinkFabricHealthMask = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: MetricNVLinkFabricHealthMask,
			Help: "NVML fabric health mask used as source input for backend health-check evaluation.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricNVLinkInactiveCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: MetricNVLinkInactiveCount,
			Help: "Count of inactive NVLinks used as source input for backend health-check evaluation.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricNVLinkLinkCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: MetricNVLinkLinkCount,
			Help: "NVLink link count used as source input for backend health-check evaluation.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricNVLinkSpeedMBytesPerSec = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: MetricNVLinkSpeedMBytesPerSec,
			Help: "NVLink common speed in MB/s used as source input for backend health-check evaluation.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricNVLinkFabricHealthMask,
		metricNVLinkInactiveCount,
		metricNVLinkLinkCount,
		metricNVLinkSpeedMBytesPerSec,
	)
}

func recordNVLinkSourceMetrics(metrics []nvlinkSourceMetric) {
	for _, metric := range metrics {
		labels := prometheus.Labels{"uuid": metric.uuid, "gpu": metric.gpu}
		switch metric.name {
		case MetricNVLinkFabricHealthMask:
			metricNVLinkFabricHealthMask.With(labels).Set(metric.value)
		case MetricNVLinkInactiveCount:
			metricNVLinkInactiveCount.With(labels).Set(metric.value)
		case MetricNVLinkLinkCount:
			metricNVLinkLinkCount.With(labels).Set(metric.value)
		case MetricNVLinkSpeedMBytesPerSec:
			metricNVLinkSpeedMBytesPerSec.With(labels).Set(metric.value)
		}
	}
}
