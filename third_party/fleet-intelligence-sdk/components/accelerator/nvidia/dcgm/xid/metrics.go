// Package xid provides DCGM XID error metrics collection and reporting.
package xid

import (
	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

// xidFields defines the DCGM fields to monitor for XID error metrics
var xidFields = []dcgm.Short{
	dcgm.DCGM_FI_DEV_XID_ERRORS, // XID errors - the value is the last XID error encountered
}

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricDCGMXIDErrors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_xid_errors",
			Help:      "Count of XID errors detected between the last check and current check.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu", "xid"},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricDCGMXIDErrors,
	)
}
