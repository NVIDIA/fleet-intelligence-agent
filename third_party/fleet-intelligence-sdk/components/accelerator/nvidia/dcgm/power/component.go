// Package power tracks NVIDIA GPU power usage via DCGM.
package power

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	"github.com/NVIDIA/fleet-intelligence-sdk/components"
	dcgmcommon "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/dcgm/common"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/eventstore"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	nvidiadcgm "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/dcgm"
	nvidianvml "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/nvml"
)

const Name = "accelerator-nvidia-dcgm-power"

const (
	defaultHealthCheckInterval = time.Minute

	// Event names for power policy violations
	EventNamePowerPolicyViolation = "power_policy_violation"

	// Legacy event name for power errors (kept for backward compatibility)
	EventNamePowerError = "power_error"

	// Default retention period for events
	DefaultRetentionPeriod = 3 * 24 * time.Hour
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	healthCheckInterval time.Duration

	dcgmInstance             nvidiadcgm.Instance
	dcgmHealthCache          *nvidiadcgm.HealthCache
	dcgmFieldValueCache      *nvidiadcgm.FieldValueCache
	dcgmPolicyViolationCache *nvidiadcgm.PolicyViolationCache
	nvmlInstance             nvidianvml.Instance
	eventBucket              eventstore.Bucket

	// Policy violation listener - receives violations from DCGM
	policyViolationCh <-chan dcgm.PolicyViolation

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

		healthCheckInterval:      healthCheckInterval,
		dcgmInstance:             gpudInstance.DCGMInstance,
		dcgmHealthCache:          gpudInstance.DCGMHealthCache,
		dcgmFieldValueCache:      gpudInstance.DCGMFieldValueCache,
		dcgmPolicyViolationCache: gpudInstance.DCGMPolicyViolationCache,
		nvmlInstance:             gpudInstance.NVMLInstance,
	}

	// Only initialize if DCGM is available
	if c.dcgmInstance != nil && c.dcgmInstance.DCGMExists() {
		// Register this component's health watch system with DCGM
		if err := c.dcgmInstance.AddHealthWatch(dcgm.DCGM_HEALTH_WATCH_POWER); err != nil {
			log.Logger.Warnw("failed to add power health watch", "error", err)
		} else {
			log.Logger.Infow("registered DCGM power health watch")
		}

		// Register power fields with DCGM instance for centralized watching
		if err := c.dcgmInstance.AddFieldsToWatch(powerFields); err != nil {
			log.Logger.Warnw("failed to register power fields", "error", err)
		} else {
			log.Logger.Infow("registered power fields for centralized watching",
				"numFields", len(powerFields))
		}

		// Setup event bucket and subscribe to power policy violations
		if gpudInstance.EventStore != nil && gpudInstance.DCGMPolicyViolationCache != nil && gpudInstance.EnableDCGMPolicy {
			// Check existing policies and register power policy if needed
			existingPolicies := c.dcgmInstance.GetExistingPolicies()
			shouldEnablePowerPolicy := false
			hadExistingPolicies := existingPolicies != nil && existingPolicies.Conditions != nil && len(existingPolicies.Conditions) > 0

			if !hadExistingPolicies {
				log.Logger.Infow("no existing policies, registering power policy")

				// Query GPU power limit using NVML
				var maxPower *uint32
				if c.nvmlInstance != nil && c.nvmlInstance.NVMLExists() {
					devices := c.nvmlInstance.Devices()
					if len(devices) > 0 {
						// Get the first device
						for _, dev := range devices {
							powerLimit, ret := dev.GetPowerManagementLimit()
							if ret == 0 { // nvml.SUCCESS
								// Convert from milliwatts to watts
								powerLimitWatts := uint32(powerLimit / 1000)
								maxPower = &powerLimitWatts
								log.Logger.Infow("using GPU power limit from NVML as threshold", "powerLimitWatts", powerLimitWatts)
								break
							}
						}
						if maxPower == nil {
							log.Logger.Warnw("failed to query GPU power limit from NVML, using default")
						}
					}
				} else {
					log.Logger.Warnw("NVML not available, using default power limit")
				}

				policyConfig := dcgm.PolicyConfig{
					Condition: dcgm.PowerPolicy,
					MaxPower:  maxPower,
				}
				gpudInstance.DCGMPolicyViolationCache.RegisterPolicyToSet(policyConfig)
				shouldEnablePowerPolicy = true
			} else {
				// Check if power policy is already configured
				if _, hasPowerPolicy := existingPolicies.Conditions[dcgm.PowerPolicy]; hasPowerPolicy {
					shouldEnablePowerPolicy = true
				} else {
					log.Logger.Infow("power policy not configured, skipping violation monitoring")
				}
			}

			// Only setup event bucket and subscribe if power policy is enabled
			if shouldEnablePowerPolicy {
				var err error
				c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
				if err != nil {
					log.Logger.Warnw("failed to create event bucket, policy violation logging disabled", "error", err)
				} else {
					// Subscribe to power policy violations from centralized cache
					c.policyViolationCh = gpudInstance.DCGMPolicyViolationCache.Subscribe("PowerPolicy")
					// Start processing violations
					c.wg.Add(1)
					go c.processPolicyViolations()
					log.Logger.Infow("power policy violation monitoring enabled")
				}
			}
		}
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
	if c.eventBucket == nil {
		return nil, nil
	}

	events, err := c.eventBucket.Get(ctx, since)
	if err != nil {
		return nil, err
	}

	// Enrich events with type and message
	var ret apiv1.Events
	for _, event := range events {
		enriched := c.enrichPowerEvent(event)
		ret = append(ret, enriched.ToEvent())
	}

	return ret, nil
}

// enrichPowerEvent adds type and message to power events and policy violations
func (c *component) enrichPowerEvent(event eventstore.Event) eventstore.Event {
	ret := event

	// Handle Power policy violations
	if event.Name == EventNamePowerPolicyViolation && event.ExtraInfo != nil {
		ret.Type = string(apiv1.EventTypeWarning) // Performance severity per DCGM spec
		powerViolation := event.ExtraInfo["power_violation"]
		ret.Message = fmt.Sprintf("Power excursion policy violation at %s (violation level: %s)",
			event.Time.Format(time.RFC3339), powerViolation)
		return ret
	}

	if event.Name == EventNamePowerError && event.ExtraInfo != nil {
		errorType := event.ExtraInfo["error_type"]

		// Determine severity based on error type
		switch errorType {
		case "power_violation", "power_limit_exceeded":
			ret.Type = string(apiv1.EventTypeCritical)
			ret.Message = fmt.Sprintf("Critical power issue (%s) detected at %s",
				errorType, event.Time.Format(time.RFC3339))
		case "power_warning":
			ret.Type = string(apiv1.EventTypeWarning)
			ret.Message = fmt.Sprintf("Power warning (%s) detected at %s",
				errorType, event.Time.Format(time.RFC3339))
		default:
			ret.Type = string(apiv1.EventTypeInfo)
			ret.Message = fmt.Sprintf("Power event (%s) detected at %s",
				errorType, event.Time.Format(time.RFC3339))
		}
	}

	return ret
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// Field watching is managed by centralized FieldValueCache, no cleanup needed here

	c.cancel()
	c.wg.Wait() // Wait for processPolicyViolations goroutine to complete
	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu power via DCGM")

	cr := &checkResult{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

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
	if c.dcgmHealthCache == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "DCGM health cache is not configured"
		return cr
	}

	// Build entity ID to UUID mapping from DCGM devices
	// This provides the mapping from entity ID (0, 1, 2, etc.) to entity UUID
	deviceMapping := make(map[uint]string)
	for _, device := range c.dcgmInstance.GetDevices() {
		deviceMapping[device.ID] = device.UUID
	}

	// Get cached DCGM power health check result from shared cache
	healthResult, incidents, err := c.dcgmHealthCache.GetResult(dcgm.DCGM_HEALTH_WATCH_POWER)
	if err != nil {
		if nvidiadcgm.IsUnhealthyAPIError(err) {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = nvidiadcgm.AppendDCGMErrorType("failed to get DCGM power health check result", err)
		} else {
			// Unknown error - treat as degraded
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = fmt.Sprintf("failed to get DCGM power health check result: %v", err)
		}
		cr.err = err
		return cr
	}

	// Query and export DCGM power field values for all devices
	// Use the convenient API that handles device iteration internally
	deviceValues, err := c.dcgmFieldValueCache.GetResult(powerFields)
	if err != nil {
		if nvidiadcgm.IsUnhealthyAPIError(err) {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = nvidiadcgm.AppendDCGMErrorType("failed to get DCGM power fields", err)
		} else {
			// Unknown error - treat as degraded
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = fmt.Sprintf("failed to get DCGM power fields: %v", err)
		}
		cr.err = err
		return cr
	} else {
		for _, deviceData := range deviceValues {
			for _, fieldValue := range deviceData.Values {
				// Use valid value
				switch fieldValue.FieldID {
			case dcgm.DCGM_FI_DEV_POWER_USAGE:
				metricDCGMFIDevPowerUsage.With(prometheus.Labels{
					"uuid":      deviceData.UUID,
					"gpu": fmt.Sprintf("%d", deviceData.DeviceID),
				}).Set(float64(fieldValue.Float64()))
			case dcgm.DCGM_FI_DEV_TOTAL_ENERGY_CONSUMPTION:
				metricDCGMFIDevTotalEnergyConsumption.With(prometheus.Labels{
					"uuid":      deviceData.UUID,
					"gpu": fmt.Sprintf("%d", deviceData.DeviceID),
				}).Set(float64(fieldValue.Int64()))
			case dcgm.DCGM_FI_DEV_ENFORCED_POWER_LIMIT:
				metricDCGMFIDevEnforcedPowerLimit.With(prometheus.Labels{
					"uuid":      deviceData.UUID,
					"gpu": fmt.Sprintf("%d", deviceData.DeviceID),
				}).Set(float64(fieldValue.Float64()))
				}
			}
		}
	}

	// Map DCGM health result to GPUd health state
	switch healthResult {
	case dcgm.DCGM_HEALTH_RESULT_PASS:
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "all power health checks passed"
	case dcgm.DCGM_HEALTH_RESULT_WARN:
		cr.health = apiv1.HealthStateTypeDegraded
		cr.enrichedIncidents = dcgmcommon.EnrichIncidents(incidents, deviceMapping)
		cr.reason = dcgmcommon.FormatIncidents("power health warning", cr.enrichedIncidents)
	case dcgm.DCGM_HEALTH_RESULT_FAIL:
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.enrichedIncidents = dcgmcommon.EnrichIncidents(incidents, deviceMapping)
		cr.reason = dcgmcommon.FormatIncidents("power health failure", cr.enrichedIncidents)
	default:
		cr.health = apiv1.HealthStateTypeDegraded
		cr.reason = "unknown health status"
	}

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	// TODO: Add DCGM-specific power data fields

	ts                time.Time
	err               error
	health            apiv1.HealthStateType
	reason            string
	enrichedIncidents []dcgmcommon.EnrichedIncident
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

	// Add enriched DCGM incidents to ExtraInfo if available
	if len(cr.enrichedIncidents) > 0 {
		if enrichedIncidentsJSON, err := json.Marshal(cr.enrichedIncidents); err == nil {
			state.ExtraInfo = map[string]string{"dcgm_incidents": string(enrichedIncidentsJSON)}
		}
	}

	return apiv1.HealthStates{state}
}

// processPolicyViolations runs in a goroutine to listen for power policy violations
func (c *component) processPolicyViolations() {
	defer c.wg.Done()

	if c.policyViolationCh == nil {
		return
	}

	log.Logger.Debugw("power policy violation processor started")
	defer log.Logger.Debugw("power policy violation processor stopped")

	for {
		select {
		case <-c.ctx.Done():
			return

		case violation, ok := <-c.policyViolationCh:
			if !ok {
				log.Logger.Warnw("Power policy violation channel closed")
				return
			}

			// Extract power violation information
			var powerViolation uint
			if powerData, ok := violation.Data.(dcgm.PowerPolicyCondition); ok {
				powerViolation = powerData.PowerViolation
			} else {
				powerViolation = 0
			}

			// Create event
			event := eventstore.Event{
				Component: Name,
				Time:      violation.Timestamp.UTC(),
				Name:      EventNamePowerPolicyViolation,
				Type:      string(apiv1.EventTypeWarning), // Performance severity per DCGM spec
				Message: fmt.Sprintf("Power excursion policy violation at %s (violation level: %d)",
					violation.Timestamp.Format(time.RFC3339), powerViolation),
				ExtraInfo: map[string]string{
					"power_violation": fmt.Sprintf("%d", powerViolation),
					"timestamp":       violation.Timestamp.Format(time.RFC3339),
				},
			}

			// Insert the event
			cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
			defer ccancel()
			if err := c.eventBucket.Insert(cctx, event); err != nil {
				log.Logger.Errorw("failed to insert power violation event", "error", err)
			}
		}
	}
}
