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

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"

	"github.com/NVIDIA/gpuhealth/internal/attestation"
	"github.com/NVIDIA/gpuhealth/internal/config"
	"github.com/NVIDIA/gpuhealth/internal/machineinfo"
)

// GetMachineID gets machine ID from system (no database dependencies)
func GetMachineID(ctx context.Context) (string, error) {
	machineID := pkghost.MachineID()
	if machineID == "" {
		// Fallback to dynamic lookup if not cached
		return pkghost.GetMachineID(ctx)
	}
	return machineID, nil
}

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
}

// Collector defines the interface for collecting health data
type Collector interface {
	Collect(ctx context.Context) (*HealthData, error)
}

// collector implements the Collector interface
type collector struct {
	config                    *config.HealthExporterConfig
	metricsStore              pkgmetrics.Store
	eventStore                eventstore.Store
	componentsRegistry        components.Registry
	nvmlInstance              nvidianvml.Instance
	attestationManager        *attestation.Manager
	lastAttestationCollection time.Time
}

// New creates a new health data collector
func New(
	config *config.HealthExporterConfig,
	metricsStore pkgmetrics.Store,
	eventStore eventstore.Store,
	componentsRegistry components.Registry,
	nvmlInstance nvidianvml.Instance,
	attestationManager *attestation.Manager,
) Collector {
	return &collector{
		config:             config,
		metricsStore:       metricsStore,
		eventStore:         eventStore,
		componentsRegistry: componentsRegistry,
		nvmlInstance:       nvmlInstance,
		attestationManager: attestationManager,
	}
}

// Collect gathers all configured health data
func (c *collector) Collect(ctx context.Context) (*HealthData, error) {
	log.Logger.Infow("Starting health data collection")

	collectionID := GenerateCollectionID()

	machineID, err := GetMachineID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine ID: %w", err)
	}

	data := &HealthData{
		CollectionID: collectionID,
		MachineID:    machineID,
		Timestamp:    time.Now().UTC(),
	}

	// Collect machine info if enabled
	if c.config.IncludeMachineInfo {
		if err := c.collectMachineInfo(data); err != nil {
			log.Logger.Errorw("Failed to collect machine info", "error", err)
		}
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
	if c.config.AttestationEnabled {
		if err := c.collectAttestationData(data); err != nil {
			log.Logger.Errorw("Failed to collect attestation data", "error", err)
		}
	}

	return data, nil
}

// collectMachineInfo collects machine hardware information
func (c *collector) collectMachineInfo(data *HealthData) error {
	if c.nvmlInstance == nil {
		return fmt.Errorf("NVML instance not available")
	}

	machineInfo, err := machineinfo.GetMachineInfo(c.nvmlInstance)
	if err != nil {
		return fmt.Errorf("failed to get machine info: %w", err)
	}

	data.MachineInfo = machineInfo
	log.Logger.Debugw("Collected machine info", "machine_info", data.MachineInfo)
	return nil
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
		var extraInfo interface{}

		if len(healthStates) > 0 {
			firstState := healthStates[0]
			health = string(firstState.Health)
			reason = firstState.Reason
			timeValue = firstState.Time
			extraInfo = firstState.ExtraInfo

			if extraInfoMap, ok := extraInfo.(map[string]interface{}); ok {
				if dataValue, exists := extraInfoMap["data"]; exists {
					extraInfo = dataValue
				} else {
					// Empty map case for extra info
					// Without this, the extra info is serialized as "map[]" which is invalid JSON/JSONB
					extraInfo = "{}"
				}
			} else {
				// Could be nil or not a map
				extraInfo = "{}"
			}
		}

		componentData[componentName] = map[string]interface{}{
			"component_name": componentName,
			"health":         health,
			"reason":         reason,
			"time":           timeValue,
			"extra_info":     extraInfo,
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
