// Package prof tracks NVIDIA GPU profiling metrics via DCGM.
package prof

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	"github.com/NVIDIA/fleet-intelligence-sdk/components"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	nvidiadcgm "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/dcgm"
)

const Name = "accelerator-nvidia-dcgm-prof"

const (
	defaultHealthCheckInterval = time.Minute
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	healthCheckInterval time.Duration

	dcgmInstance    nvidiadcgm.Instance
	dcgmHealthCache *nvidiadcgm.HealthCache

	// Track fields we're actually watching (post-validation)
	watchedFields []dcgm.Short

	// Field group handle for cleanup
	fieldGroupID dcgm.FieldHandle

	// setupDegradedReason is non-empty when field group creation or watching setup failed
	// during New(). Check() returns Degraded immediately with this reason rather than
	// querying fields that were never successfully registered.
	setupDegradedReason string

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

// fieldValidator validates fields based on hardware support
func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)

	healthCheckInterval := defaultHealthCheckInterval
	if gpudInstance.HealthCheckInterval > 0 {
		healthCheckInterval = gpudInstance.HealthCheckInterval
	}

	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		healthCheckInterval: healthCheckInterval,
		dcgmInstance:        gpudInstance.DCGMInstance,
		dcgmHealthCache:     gpudInstance.DCGMHealthCache,
	}

	// Set up DCGM field watching for profiling fields
	if c.dcgmInstance != nil && c.dcgmInstance.DCGMExists() {
		devices := c.dcgmInstance.GetDevices()
		if len(devices) == 0 {
			log.Logger.Warnw("no GPU devices found, skipping profiling field setup")
			return c, nil
		}

		// Use first device for hardware validation
		// NOTE: If fields differ per GPU, we'd need per-device validation
		deviceID := devices[0].ID

		// Create validator with hardware support check
		validator := newFieldValidator(deviceID)

		// Validate all requested profiling fields
		validFields := validator.validateFields(profFields)

		if len(validFields) == 0 {
			log.Logger.Warnw("no valid profiling fields after hardware validation",
				"requestedFields", len(profFields),
				"deviceID", deviceID,
				"suggestion", "Check if GPU supports DCP metrics (datacenter GPUs only)")
			return c, nil
		}

		log.Logger.Infow("profiling field validation complete",
			"requestedFields", len(profFields),
			"validFields", len(validFields),
			"skippedFields", len(profFields)-len(validFields),
			"deviceID", deviceID)

		// Save validated fields for query phase
		c.watchedFields = validFields

		// Create field group with ONLY hardware-validated fields
		fieldGroupName := "gpud-prof-fields"
		fieldGroupID, err := dcgm.FieldGroupCreate(fieldGroupName, validFields)
		if err != nil {
			log.Logger.Warnw("failed to create DCGM field group", "error", err)
			c.setupDegradedReason = fmt.Sprintf("failed to create DCGM profiling field group: %v", err)
			return c, nil
		}
		c.fieldGroupID = fieldGroupID

		// Setup field watching
		updateFreqMicroseconds := int64(healthCheckInterval / time.Microsecond)
		maxKeepAge := healthCheckInterval.Seconds() * 2
		maxKeepSamples := int32(3)

		err = dcgm.WatchFieldsWithGroupEx(fieldGroupID,
			c.dcgmInstance.GetGroupHandle(),
			updateFreqMicroseconds, maxKeepAge, maxKeepSamples)
		if err != nil {
			log.Logger.Warnw("failed to set up DCGM field watching", "error", err)
			c.cleanup()
			c.setupDegradedReason = fmt.Sprintf("failed to set up DCGM profiling field watching: %v", err)
			return c, nil
		}

		log.Logger.Infow("profiling field watching configured",
			"numFields", len(validFields),
			"updateFreq", healthCheckInterval,
			"maxKeepAge", maxKeepAge,
			"maxKeepSamples", maxKeepSamples)
	}

	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Tags() []string {
	return []string{
		"accelerator",
		"gpu",
		"nvidia",
		"dcgm",
		Name,
	}
}

func (c *component) IsSupported() bool {
	if c.dcgmInstance == nil {
		return false
	}
	return c.dcgmInstance.DCGMExists()
}

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(c.healthCheckInterval)
		defer ticker.Stop()

		for {
			_ = c.Check()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.lastMu.RLock()
	defer c.lastMu.RUnlock()
	if c.lastCheckResult == nil {
		return apiv1.HealthStates{{
			Time:      metav1.NewTime(time.Now().UTC()),
			Component: Name,
			Name:      Name,
			Health:    apiv1.HealthStateTypeHealthy,
			Reason:    "no data yet",
		}}
	}
	return c.lastCheckResult.HealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}

func (c *component) cleanup() {
	if c.fieldGroupID.GetHandle() != 0 {
		if err := dcgm.FieldGroupDestroy(c.fieldGroupID); err != nil {
			log.Logger.Warnw("failed to destroy field group", "error", err)
		} else {
			log.Logger.Debugw("destroyed DCGM field group for profiling")
		}
		// Zero out handle to prevent double-destroy on repeated cleanup()/Close() calls
		c.fieldGroupID = dcgm.FieldHandle{}
	}
}

func (c *component) Close() error {
	log.Logger.Debugw("closing profiling component")
	c.cleanup()
	c.cancel()
	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking profiling metrics via DCGM")

	cr := &checkResult{ts: time.Now().UTC()}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	// Early return if setup failed during New()
	if c.setupDegradedReason != "" {
		cr.health = apiv1.HealthStateTypeDegraded
		cr.reason = c.setupDegradedReason
		return cr
	}

	if c.dcgmInstance == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "DCGM instance is nil"
		return cr
	}
	if !c.dcgmInstance.DCGMExists() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "DCGM library is not loaded"
		return cr
	}
	if len(c.watchedFields) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "no profiling fields to monitor (hardware doesn't support)"
		return cr
	}

	devices := c.dcgmInstance.GetDevices()
	for _, device := range devices {
		// Query values using EntityGetLatestValues (FieldValue_v1)
		vals, err := dcgm.EntityGetLatestValues(
			dcgm.FE_GPU,     // Entity type: GPU
			device.ID,       // Entity ID
			c.watchedFields, // Only query hardware-validated fields
		)

		if err != nil {
			// Check for fatal errors that require restart
			if nvidiadcgm.IsRestartRequired(err) {
				log.Logger.Errorw("DCGM fatal error, exiting for restart",
					"component", "prof",
					"deviceID", device.ID,
					"error", err,
					"action", "systemd/k8s will restart agent and recreate DCGM resources")
				os.Exit(1)
			}
			
			// Check if this is a transient error (benign, skip)
			if nvidiadcgm.IsTransientError(err) {
				log.Logger.Infow("DCGM transient error, will retry",
					"component", "prof",
					"deviceID", device.ID,
					"error", err)
				continue
			}
			
			// For unhealthy or unknown errors, set component state
			if nvidiadcgm.IsUnhealthyAPIError(err) {
				cr.health = apiv1.HealthStateTypeUnhealthy
				cr.reason = nvidiadcgm.AppendDCGMErrorType("failed to get DCGM profiling fields", err)
			} else {
				// Unknown error - treat as degraded
				cr.health = apiv1.HealthStateTypeDegraded
				cr.reason = fmt.Sprintf("failed to get DCGM profiling fields: %v", err)
			}
			cr.err = err
			return cr
		}

		// Process each field value with type-aware sentinel checking
		for _, val := range vals {
			if isSentinel := nvidiadcgm.CheckSentinel(val,
				"deviceID", device.ID,
				"uuid", device.UUID,
			); isSentinel {
				continue
			}

			// Value is valid - export metric based on field type
			switch val.FieldID {
		case dcgm.DCGM_FI_PROF_GR_ENGINE_ACTIVE:
			metricDCGMFIProfGrEngineActive.With(prometheus.Labels{
				"uuid":      device.UUID,
				"gpu": fmt.Sprintf("%d", device.ID),
			}).Set(val.Float64())

		case dcgm.DCGM_FI_PROF_SM_ACTIVE:
			metricDCGMFIProfSmActive.With(prometheus.Labels{
				"uuid":      device.UUID,
				"gpu": fmt.Sprintf("%d", device.ID),
			}).Set(val.Float64())

		case dcgm.DCGM_FI_PROF_SM_OCCUPANCY:
			metricDCGMFIProfSmOccupancy.With(prometheus.Labels{
				"uuid":      device.UUID,
				"gpu": fmt.Sprintf("%d", device.ID),
			}).Set(val.Float64())

		case dcgm.DCGM_FI_PROF_PIPE_TENSOR_ACTIVE:
			metricDCGMFIProfPipeTensorActive.With(prometheus.Labels{
				"uuid":      device.UUID,
				"gpu": fmt.Sprintf("%d", device.ID),
			}).Set(val.Float64())

		case dcgm.DCGM_FI_PROF_PIPE_TENSOR_IMMA_ACTIVE:
			metricDCGMFIProfPipeTensorImmaActive.With(prometheus.Labels{
				"uuid":      device.UUID,
				"gpu": fmt.Sprintf("%d", device.ID),
			}).Set(val.Float64())

		case dcgm.DCGM_FI_PROF_PIPE_TENSOR_HMMA_ACTIVE:
			metricDCGMFIProfPipeTensorHmmaActive.With(prometheus.Labels{
				"uuid":      device.UUID,
				"gpu": fmt.Sprintf("%d", device.ID),
			}).Set(val.Float64())

		case dcgm.DCGM_FI_PROF_PIPE_TENSOR_DFMA_ACTIVE:
			metricDCGMFIProfPipeTensorDfmaActive.With(prometheus.Labels{
				"uuid":      device.UUID,
				"gpu": fmt.Sprintf("%d", device.ID),
			}).Set(val.Float64())

		case dcgm.DCGM_FI_PROF_PIPE_INT_ACTIVE:
			metricDCGMFIProfPipeIntActive.With(prometheus.Labels{
				"uuid":      device.UUID,
				"gpu": fmt.Sprintf("%d", device.ID),
			}).Set(val.Float64())

		case dcgm.DCGM_FI_PROF_DRAM_ACTIVE:
			metricDCGMFIProfDramActive.With(prometheus.Labels{
				"uuid":      device.UUID,
				"gpu": fmt.Sprintf("%d", device.ID),
			}).Set(val.Float64())

		case dcgm.DCGM_FI_PROF_PIPE_FP64_ACTIVE:
			metricDCGMFIProfPipeFp64Active.With(prometheus.Labels{
				"uuid":      device.UUID,
				"gpu": fmt.Sprintf("%d", device.ID),
			}).Set(val.Float64())

		case dcgm.DCGM_FI_PROF_PIPE_FP32_ACTIVE:
			metricDCGMFIProfPipeFp32Active.With(prometheus.Labels{
				"uuid":      device.UUID,
				"gpu": fmt.Sprintf("%d", device.ID),
			}).Set(val.Float64())

		case dcgm.DCGM_FI_PROF_PIPE_FP16_ACTIVE:
			metricDCGMFIProfPipeFp16Active.With(prometheus.Labels{
				"uuid":      device.UUID,
				"gpu": fmt.Sprintf("%d", device.ID),
			}).Set(val.Float64())

		case dcgm.DCGM_FI_PROF_PCIE_TX_BYTES:
			metricDCGMFIProfPcieTxBytes.With(prometheus.Labels{
				"uuid":      device.UUID,
				"gpu": fmt.Sprintf("%d", device.ID),
			}).Set(float64(val.Int64()))

		case dcgm.DCGM_FI_PROF_PCIE_RX_BYTES:
			metricDCGMFIProfPcieRxBytes.With(prometheus.Labels{
				"uuid":      device.UUID,
				"gpu": fmt.Sprintf("%d", device.ID),
			}).Set(float64(val.Int64()))

		case dcgm.DCGM_FI_PROF_NVLINK_TX_BYTES:
			metricDCGMFIProfNvlinkTxBytes.With(prometheus.Labels{
				"uuid":      device.UUID,
				"gpu": fmt.Sprintf("%d", device.ID),
			}).Set(float64(val.Int64()))

		case dcgm.DCGM_FI_PROF_NVLINK_RX_BYTES:
			metricDCGMFIProfNvlinkRxBytes.With(prometheus.Labels{
				"uuid":      device.UUID,
				"gpu": fmt.Sprintf("%d", device.ID),
			}).Set(float64(val.Int64()))
			}
		}
	}

	// Profiling metrics are informational - always return healthy
	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = "profiling metrics collected successfully"

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	ts     time.Time
	err    error
	health apiv1.HealthStateType
	reason string
}

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}
	return ""
}

func (cr *checkResult) Summary() string {
	if cr == nil {
		return ""
	}
	return cr.reason
}

func (cr *checkResult) HealthStateType() apiv1.HealthStateType {
	if cr == nil {
		return ""
	}
	return cr.health
}

func (cr *checkResult) getError() string {
	if cr == nil || cr.err == nil {
		return ""
	}
	return cr.err.Error()
}

func (cr *checkResult) HealthStates() apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{
			{
				Time:      metav1.NewTime(time.Now().UTC()),
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Time:      metav1.NewTime(cr.ts),
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Error:     cr.getError(),
		Health:    cr.health,
	}

	return apiv1.HealthStates{state}
}
