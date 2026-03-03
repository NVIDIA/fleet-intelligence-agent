// Package pcie provides DCGM PCIe metrics collection and reporting.
package pcie

import (
	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

// pcieFields defines the DCGM fields to monitor for PCIe metrics
var pcieFields = []dcgm.Short{
	dcgm.DCGM_FI_DEV_PCIE_REPLAY_COUNTER, // PCIe replay counter
}

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricDCGMFIDevPCIeReplayCounter = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_pcie_replay_counter",
			Help:      "PCIe replay counter value.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricDCGMFIDevPCIeReplayCounter,
	)
}
