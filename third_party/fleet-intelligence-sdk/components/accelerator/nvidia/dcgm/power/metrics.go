// Package power provides DCGM power metrics collection and reporting.
package power

import (
	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

// powerFields defines the DCGM fields to monitor for power metrics
var powerFields = []dcgm.Short{
	dcgm.DCGM_FI_DEV_POWER_USAGE,              // Power usage for the device in Watts
	dcgm.DCGM_FI_DEV_TOTAL_ENERGY_CONSUMPTION, // Total energy consumption for the GPU in mJ since the driver was last reloaded
	dcgm.DCGM_FI_DEV_ENFORCED_POWER_LIMIT,     // Effective power limit that the driver enforces after taking into account all limiters
}

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricDCGMFIDevPowerUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_power_usage",
			Help:      "Power usage for the device in Watts.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevTotalEnergyConsumption = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_total_energy_consumption",
			Help:      "Total energy consumption for the GPU in mJ since the driver was last reloaded.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevEnforcedPowerLimit = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_enforced_power_limit",
			Help:      "Effective power limit that the driver enforces after taking into account all limiters.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricDCGMFIDevPowerUsage,
		metricDCGMFIDevTotalEnergyConsumption,
		metricDCGMFIDevEnforcedPowerLimit,
	)
}
