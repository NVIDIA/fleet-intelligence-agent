// Package mem provides DCGM memory metrics collection and reporting.
package mem

import (
	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

// memFields defines the DCGM fields to monitor for memory metrics
var memFields = []dcgm.Short{
	dcgm.DCGM_FI_DEV_FB_FREE,                        // Free Frame Buffer in MB
	dcgm.DCGM_FI_DEV_FB_USED,                        // Used Frame Buffer in MB
	dcgm.DCGM_FI_DEV_FB_TOTAL,                       // Total Frame Buffer in MB
	dcgm.DCGM_FI_DEV_FB_USED_PERCENT,                // Percentage used of Frame Buffer: 'Used/(Total - Reserved)'
	dcgm.DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS,    // Number of remapped rows for uncorrectable errors
	dcgm.DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS,      // Number of remapped rows for correctable errors
	dcgm.DCGM_FI_DEV_ROW_REMAP_FAILURE,              // Whether remapping of rows has failed
	dcgm.DCGM_FI_DEV_ROW_REMAP_PENDING,              // Whether remapping of rows is pending
	dcgm.DCGM_FI_DEV_ECC_SBE_VOL_TOTAL,              // Total single bit volatile ECC errors
	dcgm.DCGM_FI_DEV_ECC_DBE_VOL_TOTAL,              // Total double bit volatile ECC errors
	dcgm.DCGM_FI_DEV_ECC_SBE_AGG_TOTAL,              // Total single bit aggregate (persistent) ECC errors
	dcgm.DCGM_FI_DEV_ECC_DBE_AGG_TOTAL,              // Total double bit aggregate (persistent) ECC errors
	dcgm.DCGM_FI_DEV_RETIRED_PENDING,                // Whether pages are pending retirement
	dcgm.DCGM_FI_DEV_RETIRED_DBE,                    // Retired DBE pages
	dcgm.DCGM_FI_DEV_RETIRED_SBE,                    // Retired SBE pages
	dcgm.DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_HIGH,    // Banks with high remap row availability
	dcgm.DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_LOW,     // Banks with low remap row availability
	dcgm.DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_MAX,     // Banks with max remap row availability
	dcgm.DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_NONE,    // Banks with no remap row availability
	dcgm.DCGM_FI_DEV_BANKS_REMAP_ROWS_AVAIL_PARTIAL, // Banks with partial remap row availability
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

	metricDCGMFIDevRetiredPending = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "dcgm_fi_dev_retired_pending", Help: "Number of pages pending retirement"},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevRetiredDBE = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "dcgm_fi_dev_retired_dbe", Help: "Number of retired pages because of double bit errors. Note: monotonically increasing"},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevRetiredSBE = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "dcgm_fi_dev_retired_sbe", Help: "Number of retired pages because of single bit errors. Note: monotonically increasing"},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevBanksRemapRowsAvailHigh = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "dcgm_fi_dev_banks_remap_rows_avail_high", Help: "Historical high mark of available spare memory rows per memory bank"},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevBanksRemapRowsAvailLow = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "dcgm_fi_dev_banks_remap_rows_avail_low", Help: "Historical low mark of available spare memory rows per memory bank"},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevBanksRemapRowsAvailMax = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "dcgm_fi_dev_banks_remap_rows_avail_max", Help: "Historical max available spare memory rows per memory bank"},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevBanksRemapRowsAvailNone = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "dcgm_fi_dev_banks_remap_rows_avail_none", Help: "Historical marker of memory banks with no available spare memory rows"},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)

	metricDCGMFIDevBanksRemapRowsAvailPartial = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "dcgm_fi_dev_banks_remap_rows_avail_partial", Help: "Historical mark of partial available spare memory rows per memory bank"},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid", "gpu"},
	).MustCurryWith(componentLabel)
)

func init() {
	pkgmetrics.MustRegister(
		metricDCGMFIDevFBFree,
		metricDCGMFIDevFBUsed,
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
		metricDCGMFIDevRetiredPending,
		metricDCGMFIDevRetiredDBE,
		metricDCGMFIDevRetiredSBE,
		metricDCGMFIDevBanksRemapRowsAvailHigh,
		metricDCGMFIDevBanksRemapRowsAvailLow,
		metricDCGMFIDevBanksRemapRowsAvailMax,
		metricDCGMFIDevBanksRemapRowsAvailNone,
		metricDCGMFIDevBanksRemapRowsAvailPartial,
	)
}
