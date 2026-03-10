// SPDX-FileCopyrightText: Copyright (c) 2024, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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

// Package clock provides DCGM clock metrics collection and reporting.
package clock

import (
	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

// clockFields defines the DCGM fields to monitor for clock metrics
var clockFields = []dcgm.Short{
	dcgm.DCGM_FI_DEV_SM_CLOCK,                                       // SM clock for the device
	dcgm.DCGM_FI_DEV_MEM_CLOCK,                                      // Memory clock for the device
	dcgm.DCGM_FI_DEV_CLOCKS_EVENT_REASON_HW_THERM_SLOWDOWN_NS,       // Throttling due to temperature being too high (reducing core clocks by a factor of 2 or more) in ns
	dcgm.DCGM_FI_DEV_CLOCKS_EVENT_REASON_HW_POWER_BRAKE_SLOWDOWN_NS, // Throttling due to external power brake assertion trigger (reducing core clocks by a factor of 2 or more) in ns
}

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricDCGMFIDevSMClock = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_sm_clock",
			Help:      "SM clock for the device.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevMemClock = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_mem_clock",
			Help:      "Memory clock for the device.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevClocksEventReasonHWThermSlowdownNS = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_clocks_event_reason_hw_therm_slowdown_ns",
			Help:      "Throttling due to temperature being too high (reducing core clocks by a factor of 2 or more) in ns.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevClocksEventReasonHWPowerBrakeSlowdownNS = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_clocks_event_reason_hw_power_brake_slowdown_ns",
			Help:      "Throttling due to external power brake assertion trigger (reducing core clocks by a factor of 2 or more) in ns.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricDCGMFIDevSMClock,
		metricDCGMFIDevMemClock,
		metricDCGMFIDevClocksEventReasonHWThermSlowdownNS,
		metricDCGMFIDevClocksEventReasonHWPowerBrakeSlowdownNS,
	)
}
