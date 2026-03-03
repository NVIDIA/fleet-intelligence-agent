// Package cpu tracks NVIDIA GPU CPU metrics via DCGM.
package cpu

import (
	"context"
	"fmt"
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

const Name = "accelerator-nvidia-dcgm-cpu"

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

	cpuGroupHandle dcgm.GroupHandle // Dedicated group for CPUs
	fieldGroupID   dcgm.FieldHandle

	cpuEntities []uint // Cached CPU entity IDs

	// setupDegradedReason is non-empty when CPU group creation, field group creation,
	// or field watching setup failed during New(). Check() returns Degraded immediately
	// with this reason rather than querying fields that were never successfully registered.
	setupDegradedReason string

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

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

	// Set up DCGM field watching for CPU fields
	if c.dcgmInstance != nil && c.dcgmInstance.DCGMExists() {
		// Get CPU entities first to check if CPU monitoring is available
		cpuEntities, err := dcgm.GetEntityGroupEntities(dcgm.FE_CPU)
		if err != nil {
			// CPU monitoring is optional and requires the DCGM CPU module to be loaded
			// This is expected if the module is not loaded or CPU monitoring is not supported
			log.Logger.Infow("CPU monitoring not available", "reason", err.Error())
			return c, nil
		}

		// Skip setup if no CPUs found
		if len(cpuEntities) == 0 {
			log.Logger.Infow("no CPU entities found, skipping CPU field setup")
			return c, nil
		}

		c.cpuEntities = cpuEntities

		// Create a dedicated DCGM group for CPUs
		cpuGroupHandle, err := dcgm.CreateGroup("gpud-cpu-group")
		if err != nil {
			log.Logger.Warnw("failed to create DCGM group for CPUs", "error", err)
			c.setupDegradedReason = fmt.Sprintf("failed to create DCGM CPU group: %v", err)
			return c, nil
		}
		c.cpuGroupHandle = cpuGroupHandle

		// Add CPU entities to the group
		for _, cpuID := range c.cpuEntities {
			if err := dcgm.AddEntityToGroup(cpuGroupHandle, dcgm.FE_CPU, cpuID); err != nil {
				log.Logger.Warnw("failed to add CPU to group", "cpuID", cpuID, "error", err)
			}
		}
		if len(c.cpuEntities) > 0 {
			log.Logger.Infow("added CPUs to DCGM group", "numCPUs", len(c.cpuEntities))
		}

		// Create a field group for this component's CPU fields
		fieldGroupName := "gpud-cpu-fields"
		fieldGroupID, err := dcgm.FieldGroupCreate(fieldGroupName, cpuLevelFields)
		if err != nil {
			log.Logger.Warnw("failed to create DCGM field group for CPU fields", "error", err)
			// Clean up the created group before returning
			if destroyErr := dcgm.DestroyGroup(cpuGroupHandle); destroyErr != nil {
				log.Logger.Warnw("failed to destroy DCGM group during cleanup", "error", destroyErr)
			}
			c.cpuGroupHandle = dcgm.GroupHandle{} // Reset to indicate no group created
			c.setupDegradedReason = fmt.Sprintf("failed to create DCGM CPU field group: %v", err)
			return c, nil
		}
		c.fieldGroupID = fieldGroupID

		// Set up field watching with update frequency matching health check interval
		// Use the CPU-specific group handle instead of the default GPU group
		updateFreqMicroseconds := int64(healthCheckInterval / time.Microsecond)
		maxKeepAge := healthCheckInterval.Seconds() * 2
		maxKeepSamples := 3

		err = dcgm.WatchFieldsWithGroupEx(fieldGroupID, cpuGroupHandle,
			updateFreqMicroseconds, maxKeepAge, int32(maxKeepSamples))
		if err != nil {
			log.Logger.Warnw("failed to set up DCGM field watching for CPU fields", "error", err)
			// Clean up both field group and CPU group before returning
			if destroyErr := dcgm.FieldGroupDestroy(fieldGroupID); destroyErr != nil {
				log.Logger.Warnw("failed to destroy DCGM field group during cleanup", "error", destroyErr)
			}
			if destroyErr := dcgm.DestroyGroup(cpuGroupHandle); destroyErr != nil {
				log.Logger.Warnw("failed to destroy DCGM group during cleanup", "error", destroyErr)
			}
			c.fieldGroupID = dcgm.FieldHandle{}
			c.cpuGroupHandle = dcgm.GroupHandle{}
			c.setupDegradedReason = fmt.Sprintf("failed to set up DCGM CPU field watching: %v", err)
			return c, nil
		}
		log.Logger.Infow("set up DCGM field watching for CPU fields",
			"updateFreq", healthCheckInterval, "maxKeepAge", maxKeepAge, "numFields", len(cpuLevelFields))
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

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// Clean up DCGM field group if it was created
	if c.dcgmInstance != nil && c.dcgmInstance.DCGMExists() && c.fieldGroupID.GetHandle() != 0 {
		if err := dcgm.FieldGroupDestroy(c.fieldGroupID); err != nil {
			log.Logger.Warnw("failed to destroy DCGM field group", "error", err)
		} else {
			log.Logger.Debugw("destroyed DCGM field group for CPU component")
		}
	}

	// Clean up CPU device group if it was created
	if c.dcgmInstance != nil && c.dcgmInstance.DCGMExists() && c.cpuGroupHandle.GetHandle() != 0 {
		if err := dcgm.DestroyGroup(c.cpuGroupHandle); err != nil {
			log.Logger.Warnw("failed to destroy DCGM CPU group", "error", err)
		} else {
			log.Logger.Debugw("destroyed DCGM CPU group")
		}
	}

	c.cancel()
	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu CPU metrics via DCGM")

	cr := &checkResult{
		ts: time.Now().UTC(),
	}
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

	// Skip if no CPU entities or field group wasn't created
	if len(c.cpuEntities) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "no CPU entities available"
		return cr
	}
	if c.cpuGroupHandle.GetHandle() == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "CPU group not created"
		return cr
	}

	// Query and export DCGM CPU-level field values using cached entities
	for _, cpuID := range c.cpuEntities {
		fieldValues, err := dcgm.EntityGetLatestValues(dcgm.FE_CPU, cpuID, cpuLevelFields)
		if err != nil {
			log.Logger.Warnw("failed to get CPU-level fields", "cpuID", cpuID, "error", err)
			continue
		}

		cpuIDStr := fmt.Sprintf("cpu-%d", cpuID)

		for _, fieldValue := range fieldValues {
			if isSentinel := nvidiadcgm.CheckSentinel(fieldValue, "cpuID", cpuID); isSentinel {
				continue
			}

			// Use valid value
			switch fieldValue.FieldID {
			case dcgm.DCGM_FI_DEV_CPU_TEMP_CURRENT:
				metricDCGMFIDevCPUTempCurrent.With(prometheus.Labels{"cpu_id": cpuIDStr}).Set(fieldValue.Float64())
			case dcgm.DCGM_FI_DEV_CPU_POWER_CURRENT:
				metricDCGMFIDevCPUPowerCurrent.With(prometheus.Labels{"cpu_id": cpuIDStr}).Set(fieldValue.Float64())
			case dcgm.DCGM_FI_DEV_CPU_POWER_LIMIT:
				// DCGM_FT_FP64_BLANK indicates no data available
				powerLimit := fieldValue.Float64()
				if powerLimit < dcgm.DCGM_FT_FP64_BLANK {
					metricDCGMFIDevCPUPowerLimit.With(prometheus.Labels{"cpu_id": cpuIDStr}).Set(powerLimit)
				}
			}
		}
	}

	// CPU metrics are informational only - always return healthy
	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = "CPU metrics collected successfully"

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
