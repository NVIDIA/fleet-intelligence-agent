// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package ethernet

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

const SubSystem = "network_ethernet"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricRxBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "rx_bytes",
			Help:      "Total bytes received on the interface.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "interface"},
	).MustCurryWith(componentLabel)

	metricRxPackets = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "rx_packets",
			Help:      "Total packets received on the interface.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "interface"},
	).MustCurryWith(componentLabel)

	metricRxErrors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "rx_errors",
			Help:      "Total receive errors on the interface.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "interface"},
	).MustCurryWith(componentLabel)

	metricRxDropped = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "rx_dropped",
			Help:      "Total received packets dropped on the interface.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "interface"},
	).MustCurryWith(componentLabel)

	metricTxBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "tx_bytes",
			Help:      "Total bytes transmitted on the interface.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "interface"},
	).MustCurryWith(componentLabel)

	metricTxPackets = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "tx_packets",
			Help:      "Total packets transmitted on the interface.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "interface"},
	).MustCurryWith(componentLabel)

	metricTxErrors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "tx_errors",
			Help:      "Total transmit errors on the interface.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "interface"},
	).MustCurryWith(componentLabel)

	metricTxDropped = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "tx_dropped",
			Help:      "Total transmitted packets dropped on the interface.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "interface"},
	).MustCurryWith(componentLabel)

	metricLinkUp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "link_up",
			Help:      "Current link state: 1 if link is up, 0 otherwise.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "interface"},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricRxBytes,
		metricRxPackets,
		metricRxErrors,
		metricRxDropped,
		metricTxBytes,
		metricTxPackets,
		metricTxErrors,
		metricTxDropped,
		metricLinkUp,
	)
}

// resetEthernetMetrics clears all per-interface series for ethernet metrics.
// We call this once per check so we don't need to track/dispose stale per-interface series.
func resetEthernetMetrics() {
	metricRxBytes.Reset()
	metricRxPackets.Reset()
	metricRxErrors.Reset()
	metricRxDropped.Reset()
	metricTxBytes.Reset()
	metricTxPackets.Reset()
	metricTxErrors.Reset()
	metricTxDropped.Reset()
	metricLinkUp.Reset()
}
