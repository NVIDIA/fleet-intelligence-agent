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

package dcgm

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
)

var _ Instance = &instance{}

// dcgmInitParams defines how we initialize the go-dcgm client.
type dcgmInitParams struct {
	address      string
	isUnixSocket string // "0" or "1" for go-dcgm
}

// - If DCGM_URL is set: connect via TCP to that address.
// - Otherwise: default to TCP "localhost" (default behavior).
// - If DCGM_URL_IS_UNIX_SOCKET is truthy: treat the address as a unix socket path.
func resolveInitFromEnv() dcgmInitParams {
	// DCGM_URL can be either:
	// - TCP address, optionally including port (e.g. "dcgm-service:5555")
	// - unix socket path (e.g. "/run/dcgm/dcgm.sock")
	addr := strings.TrimSpace(os.Getenv("DCGM_URL"))
	isUnixSocketRaw := strings.TrimSpace(os.Getenv("DCGM_URL_IS_UNIX_SOCKET"))
	isUnixSocket := "0"
	if isUnixSocketRaw != "" {
		parsed, err := strconv.ParseBool(isUnixSocketRaw)
		if err == nil && parsed {
			isUnixSocket = "1"
		}
	}

	if addr == "" {
		addr = "localhost"
	}

	return dcgmInitParams{address: addr, isUnixSocket: isUnixSocket}
}

// allHealthSystems lists all DCGM health systems
var allHealthSystems = []dcgm.HealthSystem{
	dcgm.DCGM_HEALTH_WATCH_PCIE,
	dcgm.DCGM_HEALTH_WATCH_NVLINK,
	dcgm.DCGM_HEALTH_WATCH_MEM,
	dcgm.DCGM_HEALTH_WATCH_INFOROM,
	dcgm.DCGM_HEALTH_WATCH_THERMAL,
	dcgm.DCGM_HEALTH_WATCH_POWER,
	dcgm.DCGM_HEALTH_WATCH_NVSWITCH_NONFATAL,
	dcgm.DCGM_HEALTH_WATCH_NVSWITCH_FATAL,
}

// DeviceInfo stores cached device information
type DeviceInfo struct {
	ID   uint
	UUID string
}

// Instance is the DCGM library connector interface.
type Instance interface {
	// DCGMExists returns true if DCGM is available.
	DCGMExists() bool

	// AddEntityToGroup adds an entity to the DCGM group.
	AddEntityToGroup(entityID uint) error

	// AddHealthWatch registers health systems to monitor.
	AddHealthWatch(system dcgm.HealthSystem) error

	// RemoveHealthWatch unregisters health systems.
	RemoveHealthWatch(system dcgm.HealthSystem) error

	// HealthCheck performs a health check for the specified system.
	HealthCheck(system dcgm.HealthSystem) (dcgm.HealthResult, []dcgm.Incident, error)

	// AddFieldsToWatch registers fields to be watched.
	AddFieldsToWatch(fields []dcgm.Short) error

	// GetWatchedFields returns all fields that have been registered.
	GetWatchedFields() []dcgm.Short

	// RemoveFieldsFromWatch unregisters fields from tracking.
	RemoveFieldsFromWatch(fields []dcgm.Short) error

	// GetLatestValuesForFields returns the latest field values for the specified device.
	// Note: This should primarily be used through FieldValueCache for better performance.
	GetLatestValuesForFields(deviceID uint, fields []dcgm.Short) ([]dcgm.FieldValue_v1, error)

	// GetGroupHandle returns the DCGM group handle for use by components.
	GetGroupHandle() dcgm.GroupHandle

	// GetDevices returns the cached list of GPU devices.
	GetDevices() []DeviceInfo

	// Shutdown shuts down the DCGM library.
	Shutdown() error
}

var newInstanceFunc = newInitializedInstance

// New creates a DCGM instance. Returns no-op instance if DCGM is unavailable.
func New() (Instance, error) {
	return newInitializedInstance()
}

// NewWithContext creates a DCGM instance with a bounded wait. If initialization
// exceeds the context deadline, it returns a no-op instance so callers can
// continue startup without blocking on slow DCGM device enumeration.
func NewWithContext(ctx context.Context) (Instance, error) {
	type result struct {
		inst Instance
		err  error
	}

	resultCh := make(chan result, 1)
	abandonCh := make(chan struct{})

	go func() {
		inst, err := newInstanceFunc()
		select {
		case resultCh <- result{inst: inst, err: err}:
		case <-abandonCh:
			if err == nil && inst != nil {
				_ = inst.Shutdown()
			}
		}
	}()

	select {
	case res := <-resultCh:
		return res.inst, res.err
	case <-ctx.Done():
		close(abandonCh)
		log.Logger.Warnw("DCGM initialization timed out, returning no-op instance", "error", ctx.Err())
		return NewNoOp(), nil
	}
}

func newInitializedInstance() (Instance, error) {
	initParams := resolveInitFromEnv()

	cleanup, err := dcgm.Init(dcgm.Standalone, initParams.address, initParams.isUnixSocket)
	if err != nil {
		log.Logger.Warnw("DCGM initialization failed, returning no-op instance", "error", err)
		return NewNoOp(), nil
	}

	log.Logger.Debugw("DCGM initialized successfully")

	// Create group with GPUs. Components add their own entities (e.g., NVSwitch).
	groupHandle, err := dcgm.NewDefaultGroup("gpud-health-monitoring")
	if err != nil {
		return nil, fmt.Errorf("failed to create custom DCGM group: %w", err)
	}

	log.Logger.Infow("created custom DCGM group for isolated health monitoring")

	// Fetch and cache device information once during initialization
	deviceIDs, err := dcgm.GetSupportedDevices()
	if err != nil {
		log.Logger.Warnw("failed to get supported devices", "error", err)
		deviceIDs = nil
	}

	var devices []DeviceInfo
	if deviceIDs != nil {
		devices = make([]DeviceInfo, 0, len(deviceIDs))
		for _, deviceID := range deviceIDs {
			deviceInfo, err := dcgm.GetDeviceInfo(deviceID)
			if err != nil {
				log.Logger.Warnw("failed to get device info, skipping device", "deviceID", deviceID, "error", err)
				continue
			}
			devices = append(devices, DeviceInfo{
				ID:   deviceID,
				UUID: deviceInfo.UUID,
			})
		}
		log.Logger.Infow("cached device information", "numDevices", len(devices))
	}

	inst := &instance{
		dcgmExists:  true,
		groupHandle: groupHandle,
		cleanup:     cleanup,
		devices:     devices,
	}

	return inst, nil
}

var _ Instance = &instance{}

// SystemHealthResult stores health check results for a system.
type SystemHealthResult struct {
	Health    dcgm.HealthResult
	Incidents []dcgm.Incident
}

type instance struct {
	dcgmExists  bool
	groupHandle dcgm.GroupHandle
	cleanup     func()

	// devices stores cached device information fetched once at initialization
	devices []DeviceInfo

	// Health watch tracking
	watchedSystemsMu sync.Mutex
	watchedSystems   dcgm.HealthSystem

	// Field watch tracking
	watchedFieldsMu sync.Mutex
	watchedFields   []dcgm.Short
}

func (inst *instance) DCGMExists() bool {
	return inst.dcgmExists
}

func (inst *instance) AddEntityToGroup(entityID uint) error {
	if err := dcgm.AddEntityToGroup(inst.groupHandle, dcgm.FE_SWITCH, entityID); err != nil {
		return fmt.Errorf("failed to add entity %d to DCGM group: %w", entityID, err)
	}
	return nil
}

func (inst *instance) GetGroupHandle() dcgm.GroupHandle {
	return inst.groupHandle
}

func (inst *instance) GetDevices() []DeviceInfo {
	return inst.devices
}

func (inst *instance) AddHealthWatch(system dcgm.HealthSystem) error {
	inst.watchedSystemsMu.Lock()
	defer inst.watchedSystemsMu.Unlock()

	newSystems := inst.watchedSystems | system

	if err := dcgm.HealthSet(inst.groupHandle, newSystems); err != nil {
		return fmt.Errorf("failed to set DCGM health watch for system 0x%x: %w", system, err)
	}

	inst.watchedSystems = newSystems
	log.Logger.Debugw("added DCGM health watch", "system", system, "totalSystems", inst.watchedSystems)
	return nil
}

func (inst *instance) RemoveHealthWatch(system dcgm.HealthSystem) error {
	inst.watchedSystemsMu.Lock()
	defer inst.watchedSystemsMu.Unlock()

	newSystems := inst.watchedSystems &^ system

	if err := dcgm.HealthSet(inst.groupHandle, newSystems); err != nil {
		return fmt.Errorf("failed to remove DCGM health watch for system 0x%x: %w", system, err)
	}

	inst.watchedSystems = newSystems
	log.Logger.Debugw("removed DCGM health watch", "system", system, "totalSystems", inst.watchedSystems)
	return nil
}

// HealthCheck performs a health check. Use DCGMHealthCache for efficient polling.
func (inst *instance) HealthCheck(system dcgm.HealthSystem) (dcgm.HealthResult, []dcgm.Incident, error) {
	healthResp, err := dcgm.HealthCheck(inst.groupHandle)
	if err != nil {
		return dcgm.DCGM_HEALTH_RESULT_FAIL, nil, fmt.Errorf("failed to perform DCGM health check: %w", err)
	}

	systemResults := make(map[dcgm.HealthSystem]SystemHealthResult)

	for _, sys := range allHealthSystems {
		systemResults[sys] = SystemHealthResult{
			Health:    dcgm.DCGM_HEALTH_RESULT_PASS,
			Incidents: nil,
		}
	}

	for _, incident := range healthResp.Incidents {
		result := systemResults[incident.System]
		result.Incidents = append(result.Incidents, incident)

		if incident.Health == dcgm.DCGM_HEALTH_RESULT_FAIL {
			result.Health = dcgm.DCGM_HEALTH_RESULT_FAIL
		} else if incident.Health == dcgm.DCGM_HEALTH_RESULT_WARN && result.Health != dcgm.DCGM_HEALTH_RESULT_FAIL {
			result.Health = dcgm.DCGM_HEALTH_RESULT_WARN
		}

		systemResults[incident.System] = result
	}

	result, exists := systemResults[system]
	if exists {
		return result.Health, result.Incidents, nil
	}
	return dcgm.DCGM_HEALTH_RESULT_PASS, nil, nil
}

// AddFieldsToWatch registers fields to be watched (tracking only).
// Field watching is set up by FieldValueCache when it's created.
func (inst *instance) AddFieldsToWatch(fields []dcgm.Short) error {
	inst.watchedFieldsMu.Lock()
	defer inst.watchedFieldsMu.Unlock()

	// Add fields, avoiding duplicates
	for _, field := range fields {
		found := false
		for _, existing := range inst.watchedFields {
			if existing == field {
				found = true
				break
			}
		}
		if !found {
			inst.watchedFields = append(inst.watchedFields, field)
		}
	}

	log.Logger.Debugw("registered fields with DCGM instance",
		"numFieldsAdded", len(fields),
		"totalFields", len(inst.watchedFields))

	return nil
}

// GetWatchedFields returns a copy of all registered fields.
// This is used by FieldValueCache to set up watching.
func (inst *instance) GetWatchedFields() []dcgm.Short {
	inst.watchedFieldsMu.Lock()
	defer inst.watchedFieldsMu.Unlock()

	// Return a defensive copy
	fieldsCopy := make([]dcgm.Short, len(inst.watchedFields))
	copy(fieldsCopy, inst.watchedFields)
	return fieldsCopy
}

// RemoveFieldsFromWatch unregisters fields from watching.
func (inst *instance) RemoveFieldsFromWatch(fields []dcgm.Short) error {
	inst.watchedFieldsMu.Lock()
	defer inst.watchedFieldsMu.Unlock()

	// Build a set of fields to remove for O(1) lookup
	toRemove := make(map[dcgm.Short]bool, len(fields))
	for _, field := range fields {
		toRemove[field] = true
	}

	// Build a new slice without the fields to remove
	newWatched := inst.watchedFields[:0]
	for _, field := range inst.watchedFields {
		if !toRemove[field] {
			newWatched = append(newWatched, field)
		}
	}
	inst.watchedFields = newWatched

	log.Logger.Debugw("unregistered fields from DCGM instance",
		"numFieldsRemoved", len(fields),
		"totalFields", len(inst.watchedFields))

	return nil
}

// GetLatestValuesForFields queries DCGM for the latest field values.
// Note: Prefer using FieldValueCache instead of calling this directly for better performance.
func (inst *instance) GetLatestValuesForFields(deviceID uint, fields []dcgm.Short) ([]dcgm.FieldValue_v1, error) {
	return dcgm.GetLatestValuesForFields(deviceID, fields)
}

func (inst *instance) Shutdown() error {
	if err := dcgm.DestroyGroup(inst.groupHandle); err != nil {
		log.Logger.Warnw("failed to destroy custom DCGM group", "error", err)
	} else {
		log.Logger.Debugw("destroyed custom DCGM group")
	}

	if inst.cleanup != nil {
		inst.cleanup()
	}
	return nil
}

var _ Instance = &noOpInstance{}

// NewNoOp creates a no-op DCGM instance.
func NewNoOp() Instance {
	return &noOpInstance{}
}

type noOpInstance struct{}

func (inst *noOpInstance) DCGMExists() bool                     { return false }
func (inst *noOpInstance) AddEntityToGroup(entityID uint) error { return nil }
func (inst *noOpInstance) GetGroupHandle() dcgm.GroupHandle {
	return dcgm.GroupHandle{}
}
func (inst *noOpInstance) GetDevices() []DeviceInfo { return nil }
func (inst *noOpInstance) AddHealthWatch(system dcgm.HealthSystem) error {
	return nil
}
func (inst *noOpInstance) RemoveHealthWatch(system dcgm.HealthSystem) error {
	return nil
}
func (inst *noOpInstance) HealthCheck(system dcgm.HealthSystem) (dcgm.HealthResult, []dcgm.Incident, error) {
	return dcgm.DCGM_HEALTH_RESULT_PASS, nil, nil
}
func (inst *noOpInstance) AddFieldsToWatch(fields []dcgm.Short) error {
	return nil
}
func (inst *noOpInstance) GetWatchedFields() []dcgm.Short {
	return nil
}
func (inst *noOpInstance) RemoveFieldsFromWatch(fields []dcgm.Short) error {
	return nil
}
func (inst *noOpInstance) GetLatestValuesForFields(deviceID uint, fields []dcgm.Short) ([]dcgm.FieldValue_v1, error) {
	return nil, fmt.Errorf("DCGM is not available")
}
func (inst *noOpInstance) Shutdown() error { return nil }
