// SPDX-FileCopyrightText: Copyright (c) 2024, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package thermal tracks NVIDIA GPU thermal/temperature metrics via DCGM.
package thermal

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

const Name = "accelerator-nvidia-dcgm-thermal"

const (
	defaultHealthCheckInterval = time.Minute

	// Event names for thermal policy violations
	EventNameThermalPolicyViolation = "thermal_policy_violation"

	// Default retention period for events (similar to SXID)
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
	}

	// Only initialize if DCGM is available
	if c.dcgmInstance != nil && c.dcgmInstance.DCGMExists() {
		// Register this component's health watch system with DCGM
		if err := c.dcgmInstance.AddHealthWatch(dcgm.DCGM_HEALTH_WATCH_THERMAL); err != nil {
			log.Logger.Warnw("failed to add thermal health watch", "error", err)
		} else {
			log.Logger.Infow("registered DCGM thermal health watch")
		}

		// Register temperature fields with DCGM instance for centralized watching
		if err := c.dcgmInstance.AddFieldsToWatch(temperatureFields); err != nil {
			log.Logger.Warnw("failed to register temperature fields", "error", err)
		} else {
			log.Logger.Infow("registered temperature fields for centralized watching",
				"numFields", len(temperatureFields))
		}

		// Setup event bucket and subscribe to thermal policy violations
		if gpudInstance.EventStore != nil && gpudInstance.DCGMPolicyViolationCache != nil && gpudInstance.EnableDCGMPolicy {
			// Check existing policies and register thermal policy if needed
			existingPolicies := c.dcgmInstance.GetExistingPolicies()
			shouldEnableThermalPolicy := false
			hadExistingPolicies := existingPolicies != nil && existingPolicies.Conditions != nil && len(existingPolicies.Conditions) > 0

			log.Logger.Infow("checking existing policies for thermal",
				"existingPolicies", existingPolicies,
				"hadExistingPolicies", hadExistingPolicies)

			if !hadExistingPolicies {
				log.Logger.Infow("no existing policies, registering thermal policy")
				policyConfig := dcgm.PolicyConfig{
					Condition: dcgm.ThermalPolicy,
				}
				gpudInstance.DCGMPolicyViolationCache.RegisterPolicyToSet(policyConfig)
				shouldEnableThermalPolicy = true
			} else {
				// Check if thermal policy is already configured
				if _, hasThermalPolicy := existingPolicies.Conditions[dcgm.ThermalPolicy]; hasThermalPolicy {
					shouldEnableThermalPolicy = true
				} else {
					log.Logger.Infow("thermal policy not configured, skipping violation monitoring")
				}
			}

			// Only setup event bucket and subscribe if thermal policy is enabled
			if shouldEnableThermalPolicy {
				var err error
				c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
				if err != nil {
					log.Logger.Warnw("failed to create event bucket, policy violation logging disabled", "error", err)
				} else {
					// Subscribe to thermal policy violations from centralized cache
					c.policyViolationCh = gpudInstance.DCGMPolicyViolationCache.Subscribe("ThermalPolicy")
					// Start processing violations
					c.wg.Add(1)
					go c.processPolicyViolations()
					log.Logger.Infow("thermal policy violation monitoring enabled")
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

	// Enrich events with type and message (similar to SXID's resolveSXIDEvent)
	var ret apiv1.Events
	for _, event := range events {
		enriched := c.enrichThermalEvent(event)
		ret = append(ret, enriched.ToEvent())
	}

	return ret, nil
}

// enrichThermalEvent adds type and message to thermal policy violation events
// Similar to SXID's resolveSXIDEvent pattern
func (c *component) enrichThermalEvent(event eventstore.Event) eventstore.Event {
	ret := event

	if event.Name == EventNameThermalPolicyViolation && event.ExtraInfo != nil {
		severityStr := event.ExtraInfo["severity"]

		// Determine event type based on severity
		// Severity thresholds: >=3 critical, >=1 warning, else info
		severity := 0
		if _, err := fmt.Sscanf(severityStr, "%d", &severity); err != nil {
			log.Logger.Warnw("failed to parse thermal violation severity, using default",
				"severity", severityStr, "error", err)
		}

		if severity >= 3 {
			ret.Type = string(apiv1.EventTypeCritical)
		} else if severity >= 1 {
			ret.Type = string(apiv1.EventTypeWarning)
		} else {
			ret.Type = string(apiv1.EventTypeInfo)
		}

		// Build human-readable message
		ret.Message = fmt.Sprintf("Thermal policy violation (severity: %s) detected at %s",
			severityStr, event.Time.Format(time.RFC3339))
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
	log.Logger.Infow("checking nvidia gpu thermal via DCGM")

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

	// Get cached DCGM thermal health check result from shared cache
	healthResult, incidents, err := c.dcgmHealthCache.GetResult(dcgm.DCGM_HEALTH_WATCH_THERMAL)
	if err != nil {
		if nvidiadcgm.IsUnhealthyAPIError(err) {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = nvidiadcgm.AppendDCGMErrorType("failed to get DCGM thermal health check result", err)
		} else {
			// Unknown error - treat as degraded
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = fmt.Sprintf("failed to get DCGM thermal health check result: %v", err)
		}
		cr.err = err
		return cr
	}

	// Query and export DCGM temperature field values for all devices
	deviceValues, err := c.dcgmFieldValueCache.GetResult(temperatureFields)
	if err != nil {
		if nvidiadcgm.IsUnhealthyAPIError(err) {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = nvidiadcgm.AppendDCGMErrorType("failed to get DCGM thermal fields", err)
		} else {
			// Unknown error - treat as degraded
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = fmt.Sprintf("failed to get DCGM thermal fields: %v", err)
		}
		cr.err = err
		return cr
	} else {
		for _, deviceData := range deviceValues {
			for _, fieldValue := range deviceData.Values {
				// Use valid value
				switch fieldValue.FieldID {
				case dcgm.DCGM_FI_DEV_GPU_TEMP:
					metricDCGMFIDevGPUTemp.With(prometheus.Labels{
						"uuid": deviceData.UUID,
						"gpu":  fmt.Sprintf("%d", deviceData.DeviceID),
					}).Set(float64(fieldValue.Int64()))
				case dcgm.DCGM_FI_DEV_MEMORY_TEMP:
					metricDCGMFIDevMemoryTemp.With(prometheus.Labels{
						"uuid": deviceData.UUID,
						"gpu":  fmt.Sprintf("%d", deviceData.DeviceID),
					}).Set(float64(fieldValue.Int64()))
				case dcgm.DCGM_FI_DEV_SLOWDOWN_TEMP:
					metricDCGMFIDevSlowdownTemp.With(prometheus.Labels{
						"uuid": deviceData.UUID,
						"gpu":  fmt.Sprintf("%d", deviceData.DeviceID),
					}).Set(float64(fieldValue.Int64()))
				case dcgm.DCGM_FI_DEV_THERMAL_VIOLATION:
					metricDCGMFIDevThermalViolation.With(prometheus.Labels{"uuid": deviceData.UUID, "gpu": fmt.Sprintf("%d", deviceData.DeviceID)}).Set(float64(fieldValue.Int64()))
				case dcgm.DCGM_FI_DEV_GPU_TEMP_LIMIT:
					metricDCGMFIDevGPUTempLimit.With(prometheus.Labels{"uuid": deviceData.UUID, "gpu": fmt.Sprintf("%d", deviceData.DeviceID)}).Set(float64(fieldValue.Int64()))
				}
			}
		}
	}

	// Map DCGM health result to GPUd health state
	switch healthResult {
	case dcgm.DCGM_HEALTH_RESULT_PASS:
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "all thermal health checks passed"
	case dcgm.DCGM_HEALTH_RESULT_WARN:
		cr.health = apiv1.HealthStateTypeDegraded
		cr.enrichedIncidents = dcgmcommon.EnrichIncidents(incidents, deviceMapping)
		cr.incidents = dcgmcommon.ToHealthStateIncidents(cr.enrichedIncidents)
		cr.reason = dcgmcommon.FormatIncidents("thermal health warning", cr.enrichedIncidents)
	case dcgm.DCGM_HEALTH_RESULT_FAIL:
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.enrichedIncidents = dcgmcommon.EnrichIncidents(incidents, deviceMapping)
		cr.incidents = dcgmcommon.ToHealthStateIncidents(cr.enrichedIncidents)
		cr.reason = dcgmcommon.FormatIncidents("thermal health failure", cr.enrichedIncidents)
	default:
		cr.health = apiv1.HealthStateTypeDegraded
		cr.reason = "unknown health status"
	}

	return cr
}

// processPolicyViolations runs in a goroutine to listen for policy violations
func (c *component) processPolicyViolations() {
	defer c.wg.Done()

	if c.policyViolationCh == nil {
		return
	}

	log.Logger.Debugw("thermal policy violation processor started")
	defer log.Logger.Debugw("thermal policy violation processor stopped")

	for {
		select {
		case <-c.ctx.Done():
			return

		case violation, ok := <-c.policyViolationCh:
			if !ok {
				log.Logger.Warnw("policy violation channel closed")
				return
			}

			// Extract severity from thermal policy condition
			var severity uint
			if thermalData, ok := violation.Data.(dcgm.ThermalPolicyCondition); ok {
				severity = thermalData.ThermalViolation
			}

			// Determine event type based on severity (same logic as enrichThermalEvent)
			var eventType string
			if severity >= 3 {
				eventType = string(apiv1.EventTypeCritical)
			} else if severity >= 1 {
				eventType = string(apiv1.EventTypeWarning)
			} else {
				eventType = string(apiv1.EventTypeInfo)
			}

			// Create event
			event := eventstore.Event{
				Component: Name,
				Time:      violation.Timestamp.UTC(),
				Name:      EventNameThermalPolicyViolation,
				Type:      eventType,
				Message: fmt.Sprintf("Thermal policy violation (severity: %d) detected at %s",
					severity, violation.Timestamp.Format(time.RFC3339)),
				ExtraInfo: map[string]string{
					"severity":  fmt.Sprintf("%d", severity),
					"timestamp": violation.Timestamp.Format(time.RFC3339),
				},
			}

			// Insert the event
			cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
			defer ccancel()
			if err := c.eventBucket.Insert(cctx, event); err != nil {
				log.Logger.Errorw("failed to insert thermal violation event", "error", err)
			}
		}
	}
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	ts                time.Time
	err               error
	health            apiv1.HealthStateType
	reason            string
	incidents         []apiv1.HealthStateIncident
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
		Incidents: cr.incidents,
	}

	// Add enriched DCGM incidents to ExtraInfo if available
	if len(cr.enrichedIncidents) > 0 {
		if enrichedIncidentsJSON, err := json.Marshal(cr.enrichedIncidents); err == nil {
			state.ExtraInfo = map[string]string{"dcgm_incidents": string(enrichedIncidentsJSON)}
		}
	}

	return apiv1.HealthStates{state}
}
