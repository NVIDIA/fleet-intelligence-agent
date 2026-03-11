// Package cpu provides DCGM CPU metrics collection and reporting.
package cpu

import (
	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

var cpuLevelFields = []dcgm.Short{
	dcgm.DCGM_FI_DEV_CPU_TEMP_CURRENT,       // 1110 - CPU
	dcgm.DCGM_FI_DEV_CPU_POWER_UTIL_CURRENT, // 1130 - CPU
	dcgm.DCGM_FI_DEV_CPU_POWER_LIMIT,        // 1131 - CPU
	dcgm.DCGM_FI_DEV_CPU_TEMP_WARNING,       // 1111 - CPU
	dcgm.DCGM_FI_DEV_CPU_TEMP_CRITICAL,      // 1112 - CPU
}

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	// CPU-level metrics (use cpu_id label)
	metricDCGMFIDevCPUTempCurrent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_cpu_temp_current",
			Help:      "CPU temperature.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "cpu_id"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevCPUPowerLimit = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_cpu_power_limit",
			Help:      "CPU power limit.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "cpu_id"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevCPUTempWarning = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "dcgm_fi_dev_cpu_temp_warning", Help: "CPU Warning Temperature"},
		[]string{pkgmetrics.MetricComponentLabelKey, "cpu_id"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevCPUTempCritical = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "dcgm_fi_dev_cpu_temp_critical", Help: "CPU Critical Temperature"},
		[]string{pkgmetrics.MetricComponentLabelKey, "cpu_id"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevCPUPowerUtilCurrent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "dcgm_fi_dev_cpu_power_util_current", Help: "CPU power utilization"},
		[]string{pkgmetrics.MetricComponentLabelKey, "cpu_id"},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricDCGMFIDevCPUTempCurrent,
		metricDCGMFIDevCPUPowerLimit,
		metricDCGMFIDevCPUTempWarning,
		metricDCGMFIDevCPUTempCritical,
		metricDCGMFIDevCPUPowerUtilCurrent,
	)
}
