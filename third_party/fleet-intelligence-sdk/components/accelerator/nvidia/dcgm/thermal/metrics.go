// Package thermal provides DCGM thermal metrics collection and reporting.
package thermal

import (
	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

// temperatureFields defines the DCGM fields to monitor for thermal metrics
var temperatureFields = []dcgm.Short{
	dcgm.DCGM_FI_DEV_GPU_TEMP,      // GPU core temperature
	dcgm.DCGM_FI_DEV_MEMORY_TEMP,   // GPU memory temperature
	dcgm.DCGM_FI_DEV_SLOWDOWN_TEMP, // Slowdown temperature for the device
	dcgm.DCGM_FI_DEV_THERMAL_VIOLATION,
	dcgm.DCGM_FI_DEV_GPU_TEMP_LIMIT,
}

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricDCGMFIDevGPUTemp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_gpu_temp",
			Help:      "Current temperature readings for the device, in degrees C.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevMemoryTemp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_memory_temp",
			Help:      "Memory temperature for the device.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevSlowdownTemp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_slowdown_temp",
			Help:      "Slowdown temperature for the device.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevThermalViolation = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "dcgm_fi_dev_thermal_violation", Help: "Thermal Violation time in ns"},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevGPUTempLimit = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "dcgm_fi_dev_gpu_temp_limit", Help: "Thermal margin temperature (distance to nearest slowdown threshold) for this GPU"},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

)

func init() {
	pkgmetrics.MustRegister(
		metricDCGMFIDevGPUTemp,
		metricDCGMFIDevMemoryTemp,
		metricDCGMFIDevSlowdownTemp,
		metricDCGMFIDevThermalViolation,
		metricDCGMFIDevGPUTempLimit,
	)
}
