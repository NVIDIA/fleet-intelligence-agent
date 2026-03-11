// Package clock provides DCGM clock metrics collection and reporting.
package clock

import (
	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

// clockFields defines the DCGM fields to monitor for clock metrics
var clockFields = []dcgm.Short{
	dcgm.DCGM_FI_DEV_SM_CLOCK,              // SM clock for the device
	dcgm.DCGM_FI_DEV_MEM_CLOCK,             // Memory clock for the device
	dcgm.DCGM_FI_DEV_CLOCKS_EVENT_REASONS,  // Clock event reasons bitmask
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

	metricDCGMFIDevClocksEventReasons = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_clocks_event_reasons",
			Help:      "Current clock event reasons (bitmask of DCGM_CLOCKS_EVENT_REASON_*)",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricDCGMFIDevSMClock,
		metricDCGMFIDevMemClock,
		metricDCGMFIDevClocksEventReasons,
	)
}
