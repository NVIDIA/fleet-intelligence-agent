// Package pcie tracks NVIDIA GPU PCIe metrics via DCGM.
package pcie

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
)

const Name = "accelerator-nvidia-dcgm-pcie"

const (
	defaultHealthCheckInterval = time.Minute

	// Event names for PCIe policy violations
	EventNamePCIePolicyViolation = "pcie_policy_violation"

	// Default retention period for events
	DefaultRetentionPeriod = 3 * 24 * time.Hour
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	healthCheckInterval      time.Duration
	dcgmInstance             nvidiadcgm.Instance
	dcgmHealthCache          *nvidiadcgm.HealthCache
	dcgmFieldValueCache      *nvidiadcgm.FieldValueCache
	dcgmPolicyViolationCache *nvidiadcgm.PolicyViolationCache
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
		ctx:                      cctx,
		cancel:                   ccancel,
		healthCheckInterval:      healthCheckInterval,
		dcgmInstance:             gpudInstance.DCGMInstance,
		dcgmHealthCache:          gpudInstance.DCGMHealthCache,
		dcgmFieldValueCache:      gpudInstance.DCGMFieldValueCache,
		dcgmPolicyViolationCache: gpudInstance.DCGMPolicyViolationCache,
	}

	// Only initialize if DCGM is available
	if c.dcgmInstance != nil && c.dcgmInstance.DCGMExists() {
		// Register this component's health watch system with DCGM
		if err := c.dcgmInstance.AddHealthWatch(dcgm.DCGM_HEALTH_WATCH_PCIE); err != nil {
			log.Logger.Warnw("failed to add PCIe health watch", "error", err)
		} else {
			log.Logger.Infow("registered DCGM PCIe health watch")
		}

		// Register PCIe fields with DCGM instance for centralized watching
		if err := c.dcgmInstance.AddFieldsToWatch(pcieFields); err != nil {
			log.Logger.Warnw("failed to register PCIe fields", "error", err)
		} else {
			log.Logger.Infow("registered PCIe fields for centralized watching",
				"numFields", len(pcieFields))
		}

		// Setup event bucket and subscribe to PCIe policy violations
		if gpudInstance.EventStore != nil && gpudInstance.DCGMPolicyViolationCache != nil && gpudInstance.EnableDCGMPolicy {
			// Check existing policies and register PCIe policy if needed
			existingPolicies := c.dcgmInstance.GetExistingPolicies()
			shouldEnablePCIePolicy := false
			hadExistingPolicies := existingPolicies != nil && existingPolicies.Conditions != nil && len(existingPolicies.Conditions) > 0

			if !hadExistingPolicies {
				log.Logger.Infow("no existing policies, registering PCIe policy")
				policyConfig := dcgm.PolicyConfig{
					Condition: dcgm.PCIePolicy,
				}
				gpudInstance.DCGMPolicyViolationCache.RegisterPolicyToSet(policyConfig)
				shouldEnablePCIePolicy = true
			} else {
				// Check if PCIe policy is already configured
				if _, hasPCIePolicy := existingPolicies.Conditions[dcgm.PCIePolicy]; hasPCIePolicy {
					shouldEnablePCIePolicy = true
				} else {
					log.Logger.Infow("PCIe policy not configured, skipping violation monitoring")
				}
			}

			// Only setup event bucket and subscribe if PCIe policy is enabled
			if shouldEnablePCIePolicy {
				var err error
				c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
				if err != nil {
					log.Logger.Warnw("failed to create event bucket, policy violation logging disabled", "error", err)
				} else {
					// Subscribe to PCIe policy violations from centralized cache
					c.policyViolationCh = gpudInstance.DCGMPolicyViolationCache.Subscribe("PCIePolicy")
					// Start processing violations
					c.wg.Add(1)
					go c.processPolicyViolations()
					log.Logger.Infow("PCIe policy violation monitoring enabled")
				}
			}
		}
	}

	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Tags() []string {
	return []string{"accelerator", "gpu", "nvidia", "dcgm", Name}
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
		enriched := c.enrichPCIeEvent(event)
		ret = append(ret, enriched.ToEvent())
	}

	return ret, nil
}

// enrichPCIeEvent adds type and message to PCIe policy violation events
func (c *component) enrichPCIeEvent(event eventstore.Event) eventstore.Event {
	ret := event

	if event.Name == EventNamePCIePolicyViolation && event.ExtraInfo != nil {
		errorType := event.ExtraInfo["error_type"]

		// All PCIe policy violations are Fatal
		ret.Type = string(apiv1.EventTypeCritical)

		// Build human-readable message
		ret.Message = fmt.Sprintf("PCIe policy violation (%s) detected at %s",
			errorType, event.Time.Format(time.RFC3339))
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
	log.Logger.Infow("checking nvidia gpu PCIe metrics via DCGM")

	cr := &checkResult{ts: time.Now().UTC()}
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

	// Get cached DCGM PCIe health check result from shared cache
	healthResult, incidents, err := c.dcgmHealthCache.GetResult(dcgm.DCGM_HEALTH_WATCH_PCIE)
	if err != nil {
		if nvidiadcgm.IsUnhealthyAPIError(err) {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = nvidiadcgm.AppendDCGMErrorType("failed to get DCGM PCIe health check result", err)
		} else {
			// Unknown error - treat as degraded
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = fmt.Sprintf("failed to get DCGM PCIe health check result: %v", err)
		}
		cr.err = err
		return cr
	}

	// Query and export DCGM PCIe field values for all devices
	deviceValues, err := c.dcgmFieldValueCache.GetResult(pcieFields)
	if err != nil {
		if nvidiadcgm.IsUnhealthyAPIError(err) {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = nvidiadcgm.AppendDCGMErrorType("failed to get DCGM PCIe fields", err)
		} else {
			// Unknown error - treat as degraded
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = fmt.Sprintf("failed to get DCGM PCIe fields: %v", err)
		}
		cr.err = err
		return cr
	} else {
		for _, deviceData := range deviceValues {
			for _, fieldValue := range deviceData.Values {
				// Use valid value
				switch fieldValue.FieldID {
			case dcgm.DCGM_FI_DEV_PCIE_REPLAY_COUNTER:
				metricDCGMFIDevPCIeReplayCounter.With(prometheus.Labels{
					"uuid":      deviceData.UUID,
					"gpu": fmt.Sprintf("%d", deviceData.DeviceID),
				}).Set(float64(fieldValue.Int64()))
				}
			}
		}
	}

	// Map DCGM health result to GPUd health state
	switch healthResult {
	case dcgm.DCGM_HEALTH_RESULT_PASS:
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "all PCIe health checks passed"
	case dcgm.DCGM_HEALTH_RESULT_WARN:
		cr.health = apiv1.HealthStateTypeDegraded
		cr.enrichedIncidents = dcgmcommon.EnrichIncidents(incidents, deviceMapping)
		cr.reason = dcgmcommon.FormatIncidents("PCIe health warning", cr.enrichedIncidents)
	case dcgm.DCGM_HEALTH_RESULT_FAIL:
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.enrichedIncidents = dcgmcommon.EnrichIncidents(incidents, deviceMapping)
		cr.reason = dcgmcommon.FormatIncidents("PCIe health failure", cr.enrichedIncidents)
	default:
		cr.health = apiv1.HealthStateTypeDegraded
		cr.reason = "unknown health status"
	}

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
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
		return apiv1.HealthStates{{
			Time:      metav1.NewTime(time.Now().UTC()),
			Component: Name,
			Name:      Name,
			Health:    apiv1.HealthStateTypeHealthy,
			Reason:    "no data yet",
		}}
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

// processPolicyViolations runs in a goroutine to listen for policy violations
func (c *component) processPolicyViolations() {
	defer c.wg.Done()

	if c.policyViolationCh == nil {
		return
	}

	log.Logger.Debugw("PCIe policy violation processor started")
	defer log.Logger.Debugw("PCIe policy violation processor stopped")

	for {
		select {
		case <-c.ctx.Done():
			return

		case violation, ok := <-c.policyViolationCh:
			if !ok {
				log.Logger.Warnw("PCIe policy violation channel closed")
				return
			}

			// Extract error information from PCI policy condition
			var errorInfo string
			if pciData, ok := violation.Data.(dcgm.PciPolicyCondition); ok {
				errorInfo = fmt.Sprintf("replay_count=%d", pciData.ReplayCounter)
			} else {
				errorInfo = "unknown"
			}

			// Create event
			event := eventstore.Event{
				Component: Name,
				Time:      violation.Timestamp.UTC(),
				Name:      EventNamePCIePolicyViolation,
				Type:      string(apiv1.EventTypeCritical), // All PCIe policy violations are Fatal
				Message: fmt.Sprintf("PCIe policy violation (pcie_error) detected at %s",
					violation.Timestamp.Format(time.RFC3339)),
				ExtraInfo: map[string]string{
					"error_type": "pcie_error",
					"error_info": errorInfo,
					"timestamp":  violation.Timestamp.Format(time.RFC3339),
				},
			}

			// Insert the event
			cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
			defer ccancel()
			if err := c.eventBucket.Insert(cctx, event); err != nil {
				log.Logger.Errorw("failed to insert PCIe violation event", "error", err)
			}
		}
	}
}
