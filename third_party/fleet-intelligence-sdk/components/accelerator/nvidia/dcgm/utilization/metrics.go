// Package utilization provides DCGM utilization metrics collection and reporting.
package utilization

import (
	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

// utilizationFields defines the DCGM fields to monitor for utilization metrics
var utilizationFields = []dcgm.Short{
	dcgm.DCGM_FI_DEV_GPU_UTIL,      // GPU Utilization
	dcgm.DCGM_FI_DEV_MEM_COPY_UTIL, // Memory Utilization
}

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricDCGMFIDevGPUUtil = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_gpu_util",
			Help:      "GPU Utilization.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevMemCopyUtil = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_mem_copy_util",
			Help:      "Memory Utilization.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricDCGMFIDevGPUUtil,
		metricDCGMFIDevMemCopyUtil,
	)
}
