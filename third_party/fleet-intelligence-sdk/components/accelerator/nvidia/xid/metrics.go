// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package xid

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

const SubSystem = "accelerator_nvidia_xid"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricXIDErrs = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "errors_total",
			Help:      "tracks the error counts per GPU UUID and XID code",
		},
		[]string{pkgmetrics.MetricComponentLabelKey,
			"uuid", // label is GPU ID
			"xid",  // label is XID error code
		},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricXIDErrs,
	)
}
