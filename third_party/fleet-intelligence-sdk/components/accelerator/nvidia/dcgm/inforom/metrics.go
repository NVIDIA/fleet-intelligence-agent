package inforom

import (
	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

var inforomFields = []dcgm.Short{
	dcgm.DCGM_FI_DEV_INFOROM_CONFIG_VALID,
}

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricDCGMFIDevInforomConfigValid = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "dcgm_fi_dev_inforom_config_valid",
			Help: "Reads the infoROM from the flash and verifies the checksums",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(metricDCGMFIDevInforomConfigValid)
}
