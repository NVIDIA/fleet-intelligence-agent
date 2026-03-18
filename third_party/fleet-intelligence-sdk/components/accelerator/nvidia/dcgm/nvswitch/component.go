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

// Package nvswitch tracks NVIDIA NVSwitch metrics via DCGM.
package nvswitch

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	"github.com/NVIDIA/fleet-intelligence-sdk/components"
	dcgmcommon "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/dcgm/common"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	nvidiadcgm "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/dcgm"
)

const Name = "accelerator-nvidia-dcgm-nvswitch"

const (
	defaultHealthCheckInterval = time.Minute
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	healthCheckInterval time.Duration
	dcgmInstance        nvidiadcgm.Instance
	dcgmHealthCache     *nvidiadcgm.HealthCache

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
		ctx:                 cctx,
		cancel:              ccancel,
		healthCheckInterval: healthCheckInterval,
		dcgmInstance:        gpudInstance.DCGMInstance,
		dcgmHealthCache:     gpudInstance.DCGMHealthCache,
	}

	// Add NVSwitch entities to DCGM group and register health watches
	// This component takes ownership of managing NVSwitch entities in DCGM
	if c.dcgmInstance != nil && c.dcgmInstance.DCGMExists() {
		// Query for NVSwitch entities in the system
		switchEntities, err := dcgm.GetEntityGroupEntities(dcgm.FE_SWITCH)
		if err != nil {
			log.Logger.Warnw("failed to get NVSwitch entities", "error", err)
		} else if len(switchEntities) > 0 {
			// Add NVSwitch entities to the DCGM group
			addedCount := 0
			failedCount := 0
			for _, switchID := range switchEntities {
				if err := c.dcgmInstance.AddEntityToGroup(switchID); err != nil {
					log.Logger.Warnw("failed to add NVSwitch to DCGM group", "switchID", switchID, "error", err)
					failedCount++
				} else {
					addedCount++
				}
			}
			log.Logger.Infow("added NVSwitch entities to DCGM group",
				"totalCount", len(switchEntities),
				"addedCount", addedCount,
				"failedCount", failedCount)
		} else {
			log.Logger.Debugw("no NVSwitch entities found in system")
		}

		// Register health watch systems (both fatal and non-fatal)
		// This registers with DCGM, and the health cache will automatically poll these systems
		healthSystems := dcgm.DCGM_HEALTH_WATCH_NVSWITCH_FATAL | dcgm.DCGM_HEALTH_WATCH_NVSWITCH_NONFATAL
		if err := c.dcgmInstance.AddHealthWatch(healthSystems); err != nil {
			log.Logger.Warnw("failed to add NVSwitch health watch", "error", err)
		} else {
			log.Logger.Infow("registered DCGM NVSwitch health watch (fatal and non-fatal)")
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
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")
	c.cancel()
	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia NVSwitch metrics via DCGM")

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

	// Check if NVSwitch entities exist in the system
	// Query DCGM directly for NVSwitch entities
	switchEntities, err := dcgm.GetEntityGroupEntities(dcgm.FE_SWITCH)
	if err != nil {
		// Error condition - failed to query for NVSwitch entities
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = nvidiadcgm.AppendDCGMErrorType("failed to query NVSwitch entities from DCGM", err)
		log.Logger.Warnw("failed to query NVSwitch entities, cannot perform health check", "error", err)
		return cr
	}

	// If no NVSwitch entities exist in system, skip health check
	if len(switchEntities) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "no NVSwitch entities found in system"
		log.Logger.Debugw("no NVSwitch entities found, skipping health check")
		return cr
	}

	log.Logger.Debugw("found NVSwitch entities", "count", len(switchEntities))

	// Get cached DCGM NVSwitch health check results (both fatal and non-fatal) from shared cache
	// Check fatal errors
	fatalHealth, fatalIncidents, err := c.dcgmHealthCache.GetResult(dcgm.DCGM_HEALTH_WATCH_NVSWITCH_FATAL)
	if err != nil {
		if nvidiadcgm.IsUnhealthyAPIError(err) {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = nvidiadcgm.AppendDCGMErrorType("failed to get DCGM NVSwitch fatal health check result", err)
		} else {
			// Unknown error - treat as degraded
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = fmt.Sprintf("failed to get DCGM NVSwitch fatal health check result: %v", err)
		}
		cr.err = err
		return cr
	}

	// Check non-fatal errors
	nonfatalHealth, nonfatalIncidents, err := c.dcgmHealthCache.GetResult(dcgm.DCGM_HEALTH_WATCH_NVSWITCH_NONFATAL)
	if err != nil {
		if nvidiadcgm.IsUnhealthyAPIError(err) {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = nvidiadcgm.AppendDCGMErrorType("failed to get DCGM NVSwitch non-fatal health check result", err)
		} else {
			// Unknown error - treat as degraded
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = fmt.Sprintf("failed to get DCGM NVSwitch non-fatal health check result: %v", err)
		}
		cr.err = err
		return cr
	}

	// Combine incidents
	allIncidents := append(fatalIncidents, nonfatalIncidents...)

	// Determine overall health (fatal takes precedence)
	overallHealth := fatalHealth
	if fatalHealth == dcgm.DCGM_HEALTH_RESULT_PASS {
		overallHealth = nonfatalHealth
	}

	// Map DCGM health result to GPUd health state
	switch overallHealth {
	case dcgm.DCGM_HEALTH_RESULT_PASS:
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = fmt.Sprintf("all NVSwitch health checks passed (%d switches found)", len(switchEntities))
	case dcgm.DCGM_HEALTH_RESULT_WARN:
		cr.health = apiv1.HealthStateTypeDegraded
		cr.enrichedIncidents = dcgmcommon.EnrichSwitchIncidents(allIncidents)
		cr.incidents = dcgmcommon.ToHealthStateIncidents(cr.enrichedIncidents)
		cr.reason = dcgmcommon.FormatIncidents("NVSwitch health warning", cr.enrichedIncidents)
	case dcgm.DCGM_HEALTH_RESULT_FAIL:
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.enrichedIncidents = dcgmcommon.EnrichSwitchIncidents(allIncidents)
		cr.incidents = dcgmcommon.ToHealthStateIncidents(cr.enrichedIncidents)
		cr.reason = dcgmcommon.FormatIncidents("NVSwitch health failure", cr.enrichedIncidents)
	default:
		cr.health = apiv1.HealthStateTypeDegraded
		cr.reason = "unknown NVSwitch health status"
	}

	return cr
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
