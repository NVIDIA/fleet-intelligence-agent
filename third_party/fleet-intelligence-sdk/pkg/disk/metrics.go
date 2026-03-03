package disk

import (
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "disk",
	}

	metricGetUsageSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "disk",
			Name:      "get_usage_seconds",
			Help:      "tracks the time taken to get disk usage",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "mount_point"}, // label is the mount point
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricGetUsageSeconds,
	)
}
