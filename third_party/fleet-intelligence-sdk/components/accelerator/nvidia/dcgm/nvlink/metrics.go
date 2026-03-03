// Package nvlink provides DCGM NVLink metrics collection and reporting.
package nvlink

import (
	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

// nvlinkFields defines the DCGM fields to monitor for NVLink metrics
var nvlinkFields = []dcgm.Short{
	dcgm.DCGM_FI_DEV_NVLINK_BANDWIDTH_TOTAL, // Total NVLink bandwidth
}

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricDCGMFIDevNvlinkBandwidthTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_nvlink_bandwidth_total",
			Help:      "Total bidirectional NVLink bandwidth across all lanes.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricDCGMFIDevNvlinkBandwidthTotal,
	)
}
