// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package fuse

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

const SubSystem = "fuse"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricConnsCongestedPct = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "connections_congested_percent_against_threshold",
			Help:      "tracks the percentage of FUSE connections that are congested",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "device_name"}, // label is device name
	).MustCurryWith(componentLabel)

	metricConnsMaxBackgroundPct = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "connections_max_background_percent_against_threshold",
			Help:      "tracks the percentage of FUSE connections that are congested",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "device_name"}, // label is device name
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricConnsCongestedPct,
		metricConnsMaxBackgroundPct,
	)
}
