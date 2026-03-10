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

// Package xid tracks NVIDIA GPU XID errors via DCGM.
package xid

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
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/eventstore"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	nvidiadcgm "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/dcgm"
)

const Name = "accelerator-nvidia-dcgm-xid"

const (
	defaultHealthCheckInterval = time.Minute

	// Event names for XID policy violations
	EventNameXIDPolicyViolation = "xid_policy_violation"

	// Legacy event name for XID errors (kept for backward compatibility)
	EventNameXIDError = "xid_error"

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
	dcgmFieldValueCache      *nvidiadcgm.FieldValueCache
	dcgmPolicyViolationCache *nvidiadcgm.PolicyViolationCache
	eventBucket              eventstore.Bucket

	// Policy violation listener - receives violations from DCGM
	policyViolationCh <-chan dcgm.PolicyViolation

	// XID-specific field group and watching
	fieldGroupID    dcgm.FieldHandle
	lastCheckTime   time.Time
	lastCheckTimeMu sync.RWMutex

	// setupDegradedReason is non-empty when field group creation or watching setup failed
	// during New(). Check() returns Degraded immediately with this reason rather than
	// querying fields that were never successfully registered.
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
		ctx:                      cctx,
		cancel:                   ccancel,
		healthCheckInterval:      healthCheckInterval,
		dcgmInstance:             gpudInstance.DCGMInstance,
		dcgmFieldValueCache:      gpudInstance.DCGMFieldValueCache,
		dcgmPolicyViolationCache: gpudInstance.DCGMPolicyViolationCache,
	}

	// Only initialize if DCGM is available
	if c.dcgmInstance != nil && c.dcgmInstance.DCGMExists() {
		// Create a dedicated field group for XID monitoring
		fieldGroupName := "gpud-xid-fields"
		fieldGroupID, err := dcgm.FieldGroupCreate(fieldGroupName, xidFields)
		if err != nil {
			log.Logger.Warnw("failed to create DCGM field group for XID fields", "error", err)
			c.setupDegradedReason = fmt.Sprintf("failed to create DCGM XID field group: %v", err)
			return c, nil
		}
		c.fieldGroupID = fieldGroupID

		// Set up field watching for XID fields
		updateFreqMicroseconds := int64(healthCheckInterval / time.Microsecond)
		maxKeepAge := healthCheckInterval.Seconds() * 3
		maxKeepSamples := int32(100) // Keep more samples for XID errors to avoid missing any

		err = dcgm.WatchFieldsWithGroupEx(fieldGroupID, c.dcgmInstance.GetGroupHandle(),
			updateFreqMicroseconds, maxKeepAge, maxKeepSamples)
		if err != nil {
			log.Logger.Warnw("failed to set up DCGM field watching for XID fields", "error", err)
			dcgm.FieldGroupDestroy(fieldGroupID)
			c.fieldGroupID = dcgm.FieldHandle{}
			c.setupDegradedReason = fmt.Sprintf("failed to set up DCGM XID field watching: %v", err)
			return c, nil
		}

		log.Logger.Infow("set up DCGM field watching for XID fields",
			"updateFreq", healthCheckInterval,
			"maxKeepAge", maxKeepAge,
			"maxKeepSamples", maxKeepSamples,
			"numFields", len(xidFields))

		// Initialize lastCheckTime to current time to start tracking from now
		c.lastCheckTime = time.Now().UTC()

		// Setup event bucket and subscribe to XID policy violations.
		// policy monitoring is enabled only when the flag is explicitly set.
		if gpudInstance.EventStore != nil && gpudInstance.DCGMPolicyViolationCache != nil && gpudInstance.EnableDCGMPolicy {
			// Check existing policies and register XID policy if needed
			existingPolicies := c.dcgmInstance.GetExistingPolicies()
			shouldEnableXidPolicy := false
			hadExistingPolicies := existingPolicies != nil && existingPolicies.Conditions != nil && len(existingPolicies.Conditions) > 0

			if !hadExistingPolicies {
				log.Logger.Infow("no existing policies, registering XID policy")
				policyConfig := dcgm.PolicyConfig{
					Condition: dcgm.XidPolicy,
				}
				gpudInstance.DCGMPolicyViolationCache.RegisterPolicyToSet(policyConfig)
				shouldEnableXidPolicy = true
			} else {
				// Check if XID policy is already configured
				if _, hasXidPolicy := existingPolicies.Conditions[dcgm.XidPolicy]; hasXidPolicy {
					shouldEnableXidPolicy = true
				} else {
					log.Logger.Infow("XID policy not configured, skipping violation monitoring")
				}
			}

			// Only setup event bucket and subscribe if XID policy is enabled
			if shouldEnableXidPolicy {
				c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
				if err != nil {
					log.Logger.Warnw("failed to create event bucket, policy violation logging disabled", "error", err)
				} else {
					// Subscribe to XID policy violations from centralized cache
					c.policyViolationCh = gpudInstance.DCGMPolicyViolationCache.Subscribe("XidPolicy")
					// Start processing violations
					c.wg.Add(1)
					go c.processPolicyViolations()
					log.Logger.Infow("XID policy violation monitoring enabled")
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
		enriched := c.enrichXIDEvent(event)
		ret = append(ret, enriched.ToEvent())
	}

	return ret, nil
}

// enrichXIDEvent adds type and message to XID events and policy violations
func (c *component) enrichXIDEvent(event eventstore.Event) eventstore.Event {
	ret := event

	// Handle XID policy violations
	if event.Name == EventNameXIDPolicyViolation && event.ExtraInfo != nil {
		ret.Type = string(apiv1.EventTypeCritical) // Fatal/Critical severity - XIDs are serious
		xidErrNum := event.ExtraInfo["xid_err_num"]
		ret.Message = fmt.Sprintf("XID policy violation at %s (XID error number: %s)",
			event.Time.Format(time.RFC3339), xidErrNum)
		return ret
	}

	if event.Name == EventNameXIDError && event.ExtraInfo != nil {
		xidCode := event.ExtraInfo["xid_code"]

		// All XID errors are considered critical as they indicate GPU hardware/driver issues
		ret.Type = string(apiv1.EventTypeCritical)
		ret.Message = fmt.Sprintf("XID error %s detected at %s",
			xidCode, event.Time.Format(time.RFC3339))
	}

	return ret
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// Clean up DCGM field group if it was created
	if c.dcgmInstance != nil && c.dcgmInstance.DCGMExists() && c.fieldGroupID.GetHandle() != 0 {
		if err := dcgm.FieldGroupDestroy(c.fieldGroupID); err != nil {
			log.Logger.Warnw("failed to destroy DCGM field group", "error", err)
		} else {
			log.Logger.Debugw("destroyed DCGM field group for XID component")
		}
	}

	c.cancel()
	c.wg.Wait() // Wait for processPolicyViolations goroutine to complete
	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu XID errors via DCGM")

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

	// Skip if field group wasn't created
	if c.fieldGroupID.GetHandle() == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "XID field group not created"
		return cr
	}

	// Get the last check time
	c.lastCheckTimeMu.RLock()
	sinceTime := c.lastCheckTime
	c.lastCheckTimeMu.RUnlock()

	// Use GetValuesSince to get all XID errors since last check
	fieldValues, nextCheckTime, err := dcgm.GetValuesSince(
		c.dcgmInstance.GetGroupHandle(),
		c.fieldGroupID,
		sinceTime,
	)
	if err != nil {
		// Check for fatal errors that require restart
		if nvidiadcgm.IsRestartRequired(err) {
			log.Logger.Errorw("DCGM fatal error, exiting for restart",
				"component", "xid",
				"error", err,
				"action", "systemd/k8s will restart agent and recreate DCGM resources")
			os.Exit(1)
		}

		// Check if this is a transient error (benign, preserve previous state)
		if nvidiadcgm.IsTransientError(err) {
			log.Logger.Infow("DCGM transient error, will retry",
				"component", "xid",
				"error", err)

			// Don't change health state - preserve previous state
			c.lastMu.RLock()
			prevResult := c.lastCheckResult
			c.lastMu.RUnlock()

			if prevResult != nil {
				// Preserve previous state by reassigning cr
				cr = prevResult
				return cr
			}
			// No previous state - default to healthy but note we're warming up
			cr.health = apiv1.HealthStateTypeHealthy
			cr.reason = "no data available yet"
			return cr
		}

		// For unhealthy or unknown errors, set component state
		if nvidiadcgm.IsUnhealthyAPIError(err) {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = nvidiadcgm.AppendDCGMErrorType("failed to get DCGM XID errors", err)
		} else {
			// Unknown error - treat as degraded
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = fmt.Sprintf("unable to query XID errors: %v", err)
		}
		cr.err = err
		return cr
	}

	// Update the last check time for next iteration
	c.lastCheckTimeMu.Lock()
	c.lastCheckTime = nextCheckTime
	c.lastCheckTimeMu.Unlock()

	log.Logger.Debugw("retrieved XID values since last check",
		"sinceTime", sinceTime,
		"nextCheckTime", nextCheckTime,
		"numValues", len(fieldValues))

	// Process field values and detect XID errors
	deviceXIDMap := make(map[string]int64) // uuid -> latest XID value
	// Track XID error counts per device and XID number: uuid -> xid -> count
	xidErrorCounts := make(map[string]map[int64]int)

	// Build maps for device UUID/ID lookup
	deviceUUIDMap := make(map[uint]string)
	uuidToDeviceID := make(map[string]uint)
	for _, device := range c.dcgmInstance.GetDevices() {
		deviceUUIDMap[device.ID] = device.UUID
		uuidToDeviceID[device.UUID] = device.ID
	}

	for _, fieldValue := range fieldValues {
		if fieldValue.FieldID != dcgm.DCGM_FI_DEV_XID_ERRORS {
			continue
		}

		if isSentinel := nvidiadcgm.CheckSentinelV2(fieldValue,
			"entityID", fieldValue.EntityID,
			"timestamp", fieldValue.TS,
		); isSentinel {
			continue
		}

		xidValue := fieldValue.Int64()
		uuid := deviceUUIDMap[uint(fieldValue.EntityID)]
		if uuid == "" {
			uuid = fmt.Sprintf("device-%d", fieldValue.EntityID)
		}

		// Update the latest XID value for this device
		deviceXIDMap[uuid] = xidValue

		// Count XID errors per device and XID number
		if xidValue > 0 {
			if xidErrorCounts[uuid] == nil {
				xidErrorCounts[uuid] = make(map[int64]int)
			}
			xidErrorCounts[uuid][xidValue]++
		}
	}

	// Reset all XID metrics before setting new values
	metricDCGMXIDErrors.Reset()

	// Set only the current XID metrics
	for uuid, xidCounts := range xidErrorCounts {
		gpuIndex := fmt.Sprintf("%d", uuidToDeviceID[uuid])
		for xidNum, count := range xidCounts {
			metricDCGMXIDErrors.With(prometheus.Labels{
				"uuid":      uuid,
				"gpu": gpuIndex,
				"xid":       fmt.Sprintf("%d", xidNum),
			}).Set(float64(count))
		}
	}

	// XID component is healthy if it can query metrics successfully
	// Other components read these metrics and decide their own health state
	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = "XID metrics exported to Prometheus"

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
		return apiv1.HealthStates{{
			Time:      metav1.NewTime(time.Now().UTC()),
			Component: Name,
			Name:      Name,
			Health:    apiv1.HealthStateTypeHealthy,
			Reason:    "no data yet",
		}}
	}

	return apiv1.HealthStates{{
		Time:      metav1.NewTime(cr.ts),
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Error:     cr.getError(),
		Health:    cr.health,
	}}
}

// processPolicyViolations runs in a goroutine to listen for XID policy violations
func (c *component) processPolicyViolations() {
	defer c.wg.Done()

	if c.policyViolationCh == nil {
		return
	}

	log.Logger.Debugw("XID policy violation processor started")
	defer log.Logger.Debugw("XID policy violation processor stopped")

	for {
		select {
		case <-c.ctx.Done():
			return

		case violation, ok := <-c.policyViolationCh:
			if !ok {
				log.Logger.Warnw("XID policy violation channel closed")
				return
			}

			// Extract XID error information
			var xidErrNum uint
			if xidData, ok := violation.Data.(dcgm.XidPolicyCondition); ok {
				xidErrNum = xidData.ErrNum
			} else {
				xidErrNum = 0
			}

			// Create event
			event := eventstore.Event{
				Component: Name,
				Time:      violation.Timestamp.UTC(),
				Name:      EventNameXIDPolicyViolation,
				Type:      string(apiv1.EventTypeCritical), // Fatal/Critical severity - XIDs are serious
				Message: fmt.Sprintf("XID policy violation at %s (XID error number: %d)",
					violation.Timestamp.Format(time.RFC3339), xidErrNum),
				ExtraInfo: map[string]string{
					"xid_err_num": fmt.Sprintf("%d", xidErrNum),
					"timestamp":   violation.Timestamp.Format(time.RFC3339),
				},
			}

			// Insert the event
			cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
			defer ccancel()
			if err := c.eventBucket.Insert(cctx, event); err != nil {
				log.Logger.Errorw("failed to insert XID violation event", "error", err)
			}
		}
	}
}
