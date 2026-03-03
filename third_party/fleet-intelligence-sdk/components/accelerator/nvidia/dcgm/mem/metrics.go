// Package mem provides DCGM memory metrics collection and reporting.
package mem

import (
	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

// memFields defines the DCGM fields to monitor for memory metrics
var memFields = []dcgm.Short{
	dcgm.DCGM_FI_DEV_FB_FREE,                     // Free Frame Buffer in MB
	dcgm.DCGM_FI_DEV_FB_USED,                     // Used Frame Buffer in MB
	dcgm.DCGM_FI_DEV_FB_RESERVED,                 // Reserved Frame Buffer in MB
	dcgm.DCGM_FI_DEV_FB_TOTAL,                    // Total Frame Buffer in MB
	dcgm.DCGM_FI_DEV_FB_USED_PERCENT,             // Percentage used of Frame Buffer: 'Used/(Total - Reserved)'
	dcgm.DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS, // Number of remapped rows for uncorrectable errors
	dcgm.DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS,   // Number of remapped rows for correctable errors
	dcgm.DCGM_FI_DEV_ROW_REMAP_FAILURE,           // Whether remapping of rows has failed
	dcgm.DCGM_FI_DEV_ROW_REMAP_PENDING,           // Whether remapping of rows is pending
	dcgm.DCGM_FI_DEV_ECC_SBE_VOL_TOTAL,           // Total single bit volatile ECC errors
	dcgm.DCGM_FI_DEV_ECC_DBE_VOL_TOTAL,           // Total double bit volatile ECC errors
	dcgm.DCGM_FI_DEV_ECC_SBE_AGG_TOTAL,           // Total single bit aggregate (persistent) ECC errors
	dcgm.DCGM_FI_DEV_ECC_DBE_AGG_TOTAL,           // Total double bit aggregate (persistent) ECC errors
}

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: Name,
	}

	metricDCGMFIDevFBFree = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_fb_free",
			Help:      "Free Frame Buffer in MB.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevFBUsed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_fb_used",
			Help:      "Used Frame Buffer in MB.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevFBReserved = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_fb_reserved",
			Help:      "Reserved Frame Buffer in MB.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevFBTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_fb_total",
			Help:      "Total Frame Buffer in MB.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevUncorrectableRemappedRows = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_uncorrectable_remapped_rows",
			Help:      "Number of remapped rows for uncorrectable errors.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevCorrectableRemappedRows = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_correctable_remapped_rows",
			Help:      "Number of remapped rows for correctable errors.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevRowRemapFailure = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_row_remap_failure",
			Help:      "Whether remapping of rows has failed (0=no failure, 1=failure).",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevFBUsedPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_fb_used_percent",
			Help:      "Percentage used of Frame Buffer: 'Used/(Total - Reserved)'. Range 0.0-1.0",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevRowRemapPending = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_row_remap_pending",
			Help:      "Whether remapping of rows is pending (0=no pending, 1=pending).",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevECCSBEVolTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_ecc_sbe_vol_total",
			Help:      "Total single bit volatile ECC errors.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevECCDBEVolTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_ecc_dbe_vol_total",
			Help:      "Total double bit volatile ECC errors.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevECCSBEAggTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_ecc_sbe_agg_total",
			Help:      "Total single bit aggregate (persistent) ECC errors. Note: monotonically increasing.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevECCDBAggTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: "",
			Name:      "dcgm_fi_dev_ecc_dbe_agg_total",
			Help:      "Total double bit aggregate (persistent) ECC errors. Note: monotonically increasing.",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricDCGMFIDevFBFree,
		metricDCGMFIDevFBUsed,
		metricDCGMFIDevFBReserved,
		metricDCGMFIDevFBTotal,
		metricDCGMFIDevFBUsedPercent,
		metricDCGMFIDevUncorrectableRemappedRows,
		metricDCGMFIDevCorrectableRemappedRows,
		metricDCGMFIDevRowRemapFailure,
		metricDCGMFIDevRowRemapPending,
		metricDCGMFIDevECCSBEVolTotal,
		metricDCGMFIDevECCDBEVolTotal,
		metricDCGMFIDevECCSBEAggTotal,
		metricDCGMFIDevECCDBAggTotal,
	)
}
