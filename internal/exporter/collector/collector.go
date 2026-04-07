// SPDX-FileCopyrightText: Copyright (c) 2025, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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

// Package collector handles health data collection from various sources
package collector

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	"github.com/NVIDIA/fleet-intelligence-sdk/components"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/eventstore"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
	nvidianvml "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/nvml"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/attestation"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/config"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/machineinfo"
)

const initialMachineInfoWait = 5 * time.Second

// GenerateCollectionID generates a unique identifier for a data collection cycle
func GenerateCollectionID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes) // crypto/rand.Read never fails with a slice
	return hex.EncodeToString(bytes)
}

// HealthData represents the collected health data
type HealthData struct {
	CollectionID    string
	MachineID       string
	Timestamp       time.Time
	MachineInfo     *machineinfo.MachineInfo
	Metrics         pkgmetrics.Metrics
	Events          eventstore.Events
	ComponentData   map[string]interface{}
	AttestationData *attestation.AttestationData
	ConfigEntries   []config.ConfigEntry
}

// Collector defines the interface for collecting health data
type Collector interface {
	Collect(ctx context.Context) (*HealthData, error)
}

// collector implements the Collector interface
type collector struct {
	config                    *config.HealthExporterConfig
	configEntries             []config.ConfigEntry // Cached config entries computed once at startup
	metricsStore              pkgmetrics.Store
	eventStore                eventstore.Store
	componentsRegistry        components.Registry
	nvmlInstance              nvidianvml.Instance
	attestationManager        *attestation.Manager
	lastAttestationCollection time.Time
	machineID                 string            // Agent's stable identity from server initialization
	dcgmGPUIndexes            map[string]string // UUID → DCGM device ID override for GPU indices
	machineInfoProvider       machineInfoProvider
}

// New creates a new health data collector
func New(
	cfg *config.HealthExporterConfig,
	fullConfig *config.Config,
	allComponentNames []string,
	metricsStore pkgmetrics.Store,
	eventStore eventstore.Store,
	componentsRegistry components.Registry,
	nvmlInstance nvidianvml.Instance,
	attestationManager *attestation.Manager,
	machineID string,
	dcgmGPUIndexes map[string]string,
) Collector {
	// Compute config entries once at startup (no dynamic config reload)
	var configEntries []config.ConfigEntry
	if fullConfig != nil {
		configEntries = fullConfig.ToConfigEntries(allComponentNames)
		log.Logger.Infow("Config entries computed at startup", "entries_count", len(configEntries))
	}

	var provider machineInfoProvider
	if cfg != nil && cfg.IncludeMachineInfo && nvmlInstance != nil {
		var opts []machineinfo.MachineInfoOption
		if len(dcgmGPUIndexes) > 0 {
			opts = append(opts, machineinfo.WithDCGMGPUIndexes(dcgmGPUIndexes))
		}
		provider = newCachedMachineInfoProvider(nvmlInstance, 0, opts...)
		provider.RefreshAsync(context.Background())
	}

	return &collector{
		config:              cfg,
		configEntries:       configEntries,
		metricsStore:        metricsStore,
		eventStore:          eventStore,
		componentsRegistry:  componentsRegistry,
		nvmlInstance:        nvmlInstance,
		attestationManager:  attestationManager,
		machineID:           machineID,
		dcgmGPUIndexes:      dcgmGPUIndexes,
		machineInfoProvider: provider,
	}
}

// Collect gathers all configured health data
func (c *collector) Collect(ctx context.Context) (*HealthData, error) {
	log.Logger.Infow("Starting health data collection")

	collectionID := GenerateCollectionID()

	// Use the machine ID provided by server initialization
	if c.machineID == "" {
		return nil, fmt.Errorf("machine ID not initialized - collector must be created with a valid machine ID")
	}

	data := &HealthData{
		CollectionID: collectionID,
		MachineID:    c.machineID,
		Timestamp:    time.Now().UTC(),
	}

	// Collect machine info if enabled
	if c.config.IncludeMachineInfo {
		c.collectMachineInfo(ctx, data)
	}

	// Collect metrics if enabled
	if c.config.IncludeMetrics {
		if err := c.collectMetrics(ctx, data); err != nil {
			log.Logger.Errorw("Failed to collect metrics", "error", err)
		}
	}

	// Collect events if enabled
	if c.config.IncludeEvents {
		if err := c.collectEvents(ctx, data); err != nil {
			log.Logger.Errorw("Failed to collect events", "error", err)
		}
	}

	// Collect component data if enabled
	if c.config.IncludeComponentData {
		if err := c.collectComponentData(data); err != nil {
			log.Logger.Errorw("Failed to collect component data", "error", err)
		}
	}

	// Collect attestation data if provider is available
	// Attestation is always enabled if manager is available
	if err := c.collectAttestationData(data); err != nil {
		log.Logger.Errorw("Failed to collect attestation data", "error", err)
	}

	// Collect config data
	if err := c.collectConfigData(data); err != nil {
		log.Logger.Errorw("Failed to collect config data", "error", err)
	}

	return data, nil
}

// collectMachineInfo reads cached machine info and triggers a best-effort refresh.
func (c *collector) collectMachineInfo(ctx context.Context, data *HealthData) {
	if c.machineInfoProvider == nil {
		return
	}

	if _, ok := c.machineInfoProvider.Get(); !ok {
		c.machineInfoProvider.WaitForInitialRefresh(ctx, initialMachineInfoWait)
	}

	if machineInfo, ok := c.machineInfoProvider.Get(); ok {
		data.MachineInfo = machineInfo
	}

	c.machineInfoProvider.RefreshAsync(ctx)
}

// collectMetrics collects metrics data from the metrics store
func (c *collector) collectMetrics(ctx context.Context, data *HealthData) error {
	if c.metricsStore == nil {
		return fmt.Errorf("metrics store not available")
	}

	since := time.Now().Add(-c.config.MetricsLookback.Duration)
	metrics, err := c.metricsStore.Read(ctx, pkgmetrics.WithSince(since))
	if err != nil {
		return fmt.Errorf("failed to read metrics: %w", err)
	}

	data.Metrics = metrics
	log.Logger.Debugw("Collected metrics", "count", len(metrics))
	return nil
}

// collectEvents collects events data from all components
func (c *collector) collectEvents(ctx context.Context, data *HealthData) error {
	if c.eventStore == nil || c.componentsRegistry == nil {
		return fmt.Errorf("event store or components registry not available")
	}

	since := time.Now().Add(-c.config.EventsLookback.Duration)
	var allEvents eventstore.Events

	components := c.componentsRegistry.All()
	if len(components) == 0 {
		return fmt.Errorf("no components found")
	}

	for _, component := range components {
		componentEvents, err := component.Events(ctx, since)
		if err != nil {
			log.Logger.Errorw("Failed to get events from component",
				"component", component.Name(), "error", err)
			continue
		}

		// Convert component events to eventstore events
		for _, event := range componentEvents {
			componentName := event.Component
			if componentName == "" {
				componentName = component.Name()
			}

			allEvents = append(allEvents, eventstore.Event{
				Component: componentName,
				Time:      event.Time.Time,
				Name:      event.Name,
				Type:      string(event.Type),
				Message:   event.Message,
				ExtraInfo: event.ExtraInfo,
			})
		}
	}

	data.Events = allEvents
	log.Logger.Debugw("Collected events", "count", len(allEvents))
	return nil
}

// collectComponentData collects health states from all components
func (c *collector) collectComponentData(data *HealthData) error {
	if c.componentsRegistry == nil {
		return fmt.Errorf("components registry not available")
	}

	componentData := make(map[string]interface{})
	components := c.componentsRegistry.All()

	for _, component := range components {
		componentName := component.Name()

		// Get health states
		healthStates := component.LastHealthStates()
		log.Logger.Debugw("Collecting health states",
			"component", componentName, "health_states", healthStates)

		health := "Unknown"
		reason := "No health data"
		var timeValue interface{}
		var extraInfo interface{} = map[string]interface{}{} // Default to empty map for JSON marshaling
		var incidents interface{} = []apiv1.HealthStateIncident{}

		if len(healthStates) > 0 {
			firstState := healthStates[0]
			health = string(firstState.Health)
			reason = firstState.Reason
			timeValue = firstState.Time
			if len(firstState.Incidents) > 0 {
				incidents = firstState.Incidents
			}

			// Handle ExtraInfo - ensure it's properly set for JSON marshaling downstream
			// ExtraInfo can be map[string]string, map[string]interface{}, or nil
			if firstState.ExtraInfo != nil {
				// Check if it's an empty map by trying common map types
				switch v := any(firstState.ExtraInfo).(type) {
				case map[string]interface{}:
					if len(v) > 0 {
						extraInfo = v
					}
				case map[string]string:
					if len(v) > 0 {
						extraInfo = v
					}
				default:
					// For other non-nil types, use as-is
					extraInfo = firstState.ExtraInfo
				}
			}
			// If ExtraInfo is nil or empty, extraInfo keeps its default empty map
		}

		componentData[componentName] = map[string]interface{}{
			"component_name": componentName,
			"health":         health,
			"reason":         reason,
			"time":           timeValue,
			"extra_info":     extraInfo,
			"incidents":      incidents,
		}
	}

	data.ComponentData = componentData
	log.Logger.Debugw("Collected component data", "count", len(componentData))
	return nil
}

// collectAttestationData collects attestation data from the attestation manager if available and updated
func (c *collector) collectAttestationData(data *HealthData) error {
	if c.attestationManager == nil {
		log.Logger.Debugw("No attestation manager available, skipping attestation data collection")
		return nil
	}

	// Get latest attestation data (success or failure info)
	attestationData := c.attestationManager.GetAttestationData()
	data.AttestationData = attestationData

	// Update collection timestamp if data was newly updated
	if c.attestationManager.IsAttestationDataUpdated(c.lastAttestationCollection) {
		c.lastAttestationCollection = time.Now().UTC()
	}

	return nil
}

// collectConfigData returns cached agent configuration entries
// Config entries are computed once at startup since there's no dynamic config reload
func (c *collector) collectConfigData(data *HealthData) error {
	if len(c.configEntries) == 0 {
		log.Logger.Debugw("No config entries available, skipping config data collection")
		return nil
	}

	// Return cached config entries (computed once at startup)
	data.ConfigEntries = c.configEntries
	return nil
}
