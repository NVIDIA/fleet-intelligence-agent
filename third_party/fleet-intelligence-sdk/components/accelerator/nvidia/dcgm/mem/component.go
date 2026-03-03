// Package mem tracks NVIDIA GPU memory metrics via DCGM.
package mem

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

const Name = "accelerator-nvidia-dcgm-mem"

const (
	defaultHealthCheckInterval = time.Minute

	// Event names for memory policy violations
	EventNameDBEPolicyViolation      = "dbe_policy_violation"
	EventNamePageRetirementViolation = "page_retirement_violation"

	// Legacy event name for memory errors (kept for backward compatibility)
	EventNameMemoryError = "memory_error"

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

	// Policy violation channels for DBE and Page Retirement policies
	dbePolicyViolationCh          <-chan dcgm.PolicyViolation
	retiredPagesPolicyViolationCh <-chan dcgm.PolicyViolation

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
		if err := c.dcgmInstance.AddHealthWatch(dcgm.DCGM_HEALTH_WATCH_MEM); err != nil {
			log.Logger.Warnw("failed to add memory health watch", "error", err)
		} else {
			log.Logger.Infow("registered DCGM memory health watch")
		}

		// Register memory fields with DCGM instance for centralized watching
		if err := c.dcgmInstance.AddFieldsToWatch(memFields); err != nil {
			log.Logger.Warnw("failed to register memory fields", "error", err)
		} else {
			log.Logger.Infow("registered memory fields for centralized watching",
				"numFields", len(memFields))
		}

		// Setup event bucket and subscribe to memory policy violations (DBE + Page Retirement)
		if gpudInstance.EventStore != nil && gpudInstance.DCGMPolicyViolationCache != nil && gpudInstance.EnableDCGMPolicy {
			// Check existing policies and register memory policies if needed
			existingPolicies := c.dcgmInstance.GetExistingPolicies()
			shouldEnableDBEPolicy := false
			shouldEnableRetiredPagesPolicy := false
			hadExistingPolicies := existingPolicies != nil && existingPolicies.Conditions != nil && len(existingPolicies.Conditions) > 0

			if !hadExistingPolicies {
				log.Logger.Infow("no existing policies, registering memory policies")
				dbePolicyConfig := dcgm.PolicyConfig{
					Condition: dcgm.DbePolicy,
				}
				retiredPagesPolicyConfig := dcgm.PolicyConfig{
					Condition: dcgm.MaxRtPgPolicy,
				}
				gpudInstance.DCGMPolicyViolationCache.RegisterPolicyToSet(dbePolicyConfig)
				gpudInstance.DCGMPolicyViolationCache.RegisterPolicyToSet(retiredPagesPolicyConfig)
				shouldEnableDBEPolicy = true
				shouldEnableRetiredPagesPolicy = true
			} else {
				// Check if DBE policy is already configured
				if _, hasDBEPolicy := existingPolicies.Conditions[dcgm.DbePolicy]; hasDBEPolicy {
					shouldEnableDBEPolicy = true
				}
				// Check if MaxRtPgPolicy is already configured
				if _, hasMaxRtPgPolicy := existingPolicies.Conditions[dcgm.MaxRtPgPolicy]; hasMaxRtPgPolicy {
					shouldEnableRetiredPagesPolicy = true
				}
				if !shouldEnableDBEPolicy && !shouldEnableRetiredPagesPolicy {
					log.Logger.Infow("memory policies not configured, skipping violation monitoring")
				}
			}

			// Only setup event bucket and subscribe if at least one policy is enabled
			if shouldEnableDBEPolicy || shouldEnableRetiredPagesPolicy {
				var err error
				c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
				if err != nil {
					log.Logger.Warnw("failed to create event bucket, policy violation logging disabled", "error", err)
				} else {
					// Subscribe to enabled memory policy violations
					if shouldEnableDBEPolicy {
						c.dbePolicyViolationCh = gpudInstance.DCGMPolicyViolationCache.Subscribe("DbePolicy")
					}
					if shouldEnableRetiredPagesPolicy {
						c.retiredPagesPolicyViolationCh = gpudInstance.DCGMPolicyViolationCache.Subscribe("MaxRtPgPolicy")
					}
					// Start processing violations
					c.wg.Add(1)
					go c.processPolicyViolations()
					log.Logger.Infow("memory policy violation monitoring enabled")
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
		enriched := c.enrichMemoryEvent(event)
		ret = append(ret, enriched.ToEvent())
	}

	return ret, nil
}

// enrichMemoryEvent adds type and message to memory error events and policy violations
func (c *component) enrichMemoryEvent(event eventstore.Event) eventstore.Event {
	ret := event

	// Handle DBE policy violations
	if event.Name == EventNameDBEPolicyViolation && event.ExtraInfo != nil {
		ret.Type = string(apiv1.EventTypeCritical) // Fatal severity per DCGM spec
		location := event.ExtraInfo["location"]
		numErrors := event.ExtraInfo["num_errors"]
		ret.Message = fmt.Sprintf("DBE (Double-bit ECC error) policy violation at %s (location: %s, errors: %s)",
			event.Time.Format(time.RFC3339), location, numErrors)
		return ret
	}

	// Handle Page Retirement policy violations
	if event.Name == EventNamePageRetirementViolation && event.ExtraInfo != nil {
		ret.Type = string(apiv1.EventTypeWarning) // Non-Fatal severity per DCGM spec
		retiredPages := event.ExtraInfo["retired_pages"]
		sbePages := event.ExtraInfo["sbe_pages"]
		dbePages := event.ExtraInfo["dbe_pages"]
		ret.Message = fmt.Sprintf("Page retirement limit exceeded at %s (total: %s, SBE: %s, DBE: %s)",
			event.Time.Format(time.RFC3339), retiredPages, sbePages, dbePages)
		return ret
	}

	if event.Name == EventNameMemoryError && event.ExtraInfo != nil {
		errorType := event.ExtraInfo["error_type"]

		// Determine severity based on error type
		switch errorType {
		case "uncorrectable_remapped_rows", "row_remap_failure":
			ret.Type = string(apiv1.EventTypeCritical)
			ret.Message = fmt.Sprintf("Critical memory error (%s) detected at %s",
				errorType, event.Time.Format(time.RFC3339))
		case "correctable_remapped_rows":
			ret.Type = string(apiv1.EventTypeWarning)
			ret.Message = fmt.Sprintf("Memory warning (%s) detected at %s",
				errorType, event.Time.Format(time.RFC3339))
		default:
			ret.Type = string(apiv1.EventTypeInfo)
			ret.Message = fmt.Sprintf("Memory event (%s) detected at %s",
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
	log.Logger.Infow("checking nvidia gpu memory via DCGM")

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

	// Get cached DCGM memory health check result from shared cache
	healthResult, incidents, err := c.dcgmHealthCache.GetResult(dcgm.DCGM_HEALTH_WATCH_MEM)
	if err != nil {
		if nvidiadcgm.IsUnhealthyAPIError(err) {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = nvidiadcgm.AppendDCGMErrorType("failed to get DCGM memory health check result", err)
		} else {
			// Unknown error - treat as degraded
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = fmt.Sprintf("failed to get DCGM memory health check result: %v", err)
		}
		cr.err = err
		return cr
	}

	// Query and export DCGM memory field values for all devices
	deviceValues, err := c.dcgmFieldValueCache.GetResult(memFields)
	if err != nil {
		if nvidiadcgm.IsUnhealthyAPIError(err) {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = nvidiadcgm.AppendDCGMErrorType("failed to get DCGM memory fields", err)
		} else {
			// Unknown error - treat as degraded
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = fmt.Sprintf("failed to get DCGM memory fields: %v", err)
		}
		cr.err = err
		return cr
	} else {
		for _, deviceData := range deviceValues {
			for _, fieldValue := range deviceData.Values {
				// Use valid value
				switch fieldValue.FieldID {
			case dcgm.DCGM_FI_DEV_FB_FREE:
				metricDCGMFIDevFBFree.With(prometheus.Labels{
					"uuid":      deviceData.UUID,
					"gpu": fmt.Sprintf("%d", deviceData.DeviceID),
				}).Set(float64(fieldValue.Int64()))
			case dcgm.DCGM_FI_DEV_FB_USED:
				metricDCGMFIDevFBUsed.With(prometheus.Labels{
					"uuid":      deviceData.UUID,
					"gpu": fmt.Sprintf("%d", deviceData.DeviceID),
				}).Set(float64(fieldValue.Int64()))
			case dcgm.DCGM_FI_DEV_FB_RESERVED:
				metricDCGMFIDevFBReserved.With(prometheus.Labels{
					"uuid":      deviceData.UUID,
					"gpu": fmt.Sprintf("%d", deviceData.DeviceID),
				}).Set(float64(fieldValue.Int64()))
			case dcgm.DCGM_FI_DEV_FB_TOTAL:
				metricDCGMFIDevFBTotal.With(prometheus.Labels{
					"uuid":      deviceData.UUID,
					"gpu": fmt.Sprintf("%d", deviceData.DeviceID),
				}).Set(float64(fieldValue.Int64()))
			case dcgm.DCGM_FI_DEV_FB_USED_PERCENT:
				metricDCGMFIDevFBUsedPercent.With(prometheus.Labels{
					"uuid":      deviceData.UUID,
					"gpu": fmt.Sprintf("%d", deviceData.DeviceID),
				}).Set(float64(fieldValue.Float64()))
			case dcgm.DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS:
				metricDCGMFIDevUncorrectableRemappedRows.With(prometheus.Labels{
					"uuid":      deviceData.UUID,
					"gpu": fmt.Sprintf("%d", deviceData.DeviceID),
				}).Set(float64(fieldValue.Int64()))
			case dcgm.DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS:
				metricDCGMFIDevCorrectableRemappedRows.With(prometheus.Labels{
					"uuid":      deviceData.UUID,
					"gpu": fmt.Sprintf("%d", deviceData.DeviceID),
				}).Set(float64(fieldValue.Int64()))
			case dcgm.DCGM_FI_DEV_ROW_REMAP_FAILURE:
				metricDCGMFIDevRowRemapFailure.With(prometheus.Labels{
					"uuid":      deviceData.UUID,
					"gpu": fmt.Sprintf("%d", deviceData.DeviceID),
				}).Set(float64(fieldValue.Int64()))
			case dcgm.DCGM_FI_DEV_ROW_REMAP_PENDING:
				metricDCGMFIDevRowRemapPending.With(prometheus.Labels{
					"uuid":      deviceData.UUID,
					"gpu": fmt.Sprintf("%d", deviceData.DeviceID),
				}).Set(float64(fieldValue.Int64()))
			case dcgm.DCGM_FI_DEV_ECC_SBE_VOL_TOTAL:
				metricDCGMFIDevECCSBEVolTotal.With(prometheus.Labels{
					"uuid":      deviceData.UUID,
					"gpu": fmt.Sprintf("%d", deviceData.DeviceID),
				}).Set(float64(fieldValue.Int64()))
			case dcgm.DCGM_FI_DEV_ECC_DBE_VOL_TOTAL:
				metricDCGMFIDevECCDBEVolTotal.With(prometheus.Labels{
					"uuid":      deviceData.UUID,
					"gpu": fmt.Sprintf("%d", deviceData.DeviceID),
				}).Set(float64(fieldValue.Int64()))
			case dcgm.DCGM_FI_DEV_ECC_SBE_AGG_TOTAL:
				metricDCGMFIDevECCSBEAggTotal.With(prometheus.Labels{
					"uuid":      deviceData.UUID,
					"gpu": fmt.Sprintf("%d", deviceData.DeviceID),
				}).Set(float64(fieldValue.Int64()))
			case dcgm.DCGM_FI_DEV_ECC_DBE_AGG_TOTAL:
				metricDCGMFIDevECCDBAggTotal.With(prometheus.Labels{
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
		cr.reason = "all memory health checks passed"
	case dcgm.DCGM_HEALTH_RESULT_WARN:
		cr.health = apiv1.HealthStateTypeDegraded
		cr.enrichedIncidents = dcgmcommon.EnrichIncidents(incidents, deviceMapping)
		cr.reason = dcgmcommon.FormatIncidents("memory health warning", cr.enrichedIncidents)
	case dcgm.DCGM_HEALTH_RESULT_FAIL:
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.enrichedIncidents = dcgmcommon.EnrichIncidents(incidents, deviceMapping)
		cr.reason = dcgmcommon.FormatIncidents("memory health failure", cr.enrichedIncidents)
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

// processPolicyViolations runs in a goroutine to listen for memory policy violations
// This handles both DBE and Page Retirement policy violations from separate channels
func (c *component) processPolicyViolations() {
	defer c.wg.Done()

	if c.dbePolicyViolationCh == nil && c.retiredPagesPolicyViolationCh == nil {
		return
	}

	log.Logger.Debugw("memory policy violation processor started")
	defer log.Logger.Debugw("memory policy violation processor stopped")

	for {
		select {
		case <-c.ctx.Done():
			return

		case violation, ok := <-c.dbePolicyViolationCh:
			if !ok {
				log.Logger.Warnw("DBE policy violation channel closed")
				c.dbePolicyViolationCh = nil
				if c.retiredPagesPolicyViolationCh == nil {
					return
				}
				continue
			}
			c.handlePolicyViolation(violation)

		case violation, ok := <-c.retiredPagesPolicyViolationCh:
			if !ok {
				log.Logger.Warnw("Retired pages policy violation channel closed")
				c.retiredPagesPolicyViolationCh = nil
				if c.dbePolicyViolationCh == nil {
					return
				}
				continue
			}
			c.handlePolicyViolation(violation)
		}
	}
}

func (c *component) handlePolicyViolation(violation dcgm.PolicyViolation) {
	var event eventstore.Event

	// Handle different policy violation types based on the data type
	switch data := violation.Data.(type) {
	case dcgm.DbePolicyCondition:
		// DBE (Double-bit ECC) violation
		event = eventstore.Event{
			Component: Name,
			Time:      violation.Timestamp.UTC(),
			Name:      EventNameDBEPolicyViolation,
			Type:      string(apiv1.EventTypeCritical), // Fatal severity per DCGM spec
			Message: fmt.Sprintf("DBE (Double-bit ECC error) policy violation at %s (location: %s, errors: %d)",
				violation.Timestamp.Format(time.RFC3339), data.Location, data.NumErrors),
			ExtraInfo: map[string]string{
				"location":   data.Location,
				"num_errors": fmt.Sprintf("%d", data.NumErrors),
				"timestamp":  violation.Timestamp.Format(time.RFC3339),
			},
		}

	case dcgm.RetiredPagesPolicyCondition:
		// Page Retirement violation
		totalRetiredPages := data.SbePages + data.DbePages

		event = eventstore.Event{
			Component: Name,
			Time:      violation.Timestamp.UTC(),
			Name:      EventNamePageRetirementViolation,
			Type:      string(apiv1.EventTypeWarning), // Non-Fatal severity per DCGM spec
			Message: fmt.Sprintf("Page retirement limit exceeded at %s (total: %d, SBE: %d, DBE: %d)",
				violation.Timestamp.Format(time.RFC3339), totalRetiredPages, data.SbePages, data.DbePages),
			ExtraInfo: map[string]string{
				"sbe_pages":     fmt.Sprintf("%d", data.SbePages),
				"dbe_pages":     fmt.Sprintf("%d", data.DbePages),
				"retired_pages": fmt.Sprintf("%d", totalRetiredPages),
				"timestamp":     violation.Timestamp.Format(time.RFC3339),
			},
		}

	default:
		log.Logger.Warnw("unknown memory policy violation type",
			"timestamp", violation.Timestamp,
			"data_type", fmt.Sprintf("%T", data))
		return
	}

	// Insert the event
	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	defer ccancel()

	if err := c.eventBucket.Insert(cctx, event); err != nil {
		log.Logger.Errorw("failed to insert memory policy violation event", "error", err, "event_name", event.Name)
	}
}
