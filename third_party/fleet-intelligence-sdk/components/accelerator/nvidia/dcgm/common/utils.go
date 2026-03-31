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

// Package common provides common utilities for DCGM health monitoring components.
package common

import (
	"context"
	"fmt"
	"time"

	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/eventstore"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
)

// Extra-info key constants for DCGM health incident events.
const (
	EventKeyEntityID  = "entity_id"
	EventKeyErrorCode = "error_code"
)

// EmitNewIncidentEvents inserts an event into eventBucket for each incident in curr
// that is not already present in prev (onset detection). It uses EntityID+Severity+Error
// as the deduplication key to identify the same fault across check cycles.
// eventName should be a component-specific constant (e.g. "dcgm_thermal_incident").
// Errors are logged as warnings and do not propagate to the caller.
func EmitNewIncidentEvents(
	ctx context.Context,
	now time.Time,
	componentName string,
	eventName string,
	eventBucket eventstore.Bucket,
	prev []apiv1.HealthStateIncident,
	curr []apiv1.HealthStateIncident,
) {
	if eventBucket == nil || len(curr) == 0 {
		return
	}

	// Build dedup set from previous incidents.
	prevKeys := make(map[string]struct{}, len(prev))
	for _, inc := range prev {
		prevKeys[inc.EntityID+"/"+string(inc.Severity)+"/"+inc.Error] = struct{}{}
	}

	for _, inc := range curr {
		key := inc.EntityID + "/" + string(inc.Severity) + "/" + inc.Error
		if _, seen := prevKeys[key]; seen {
			continue
		}

		eventType := apiv1.EventTypeWarning
		if inc.Severity == apiv1.HealthStateTypeUnhealthy {
			eventType = apiv1.EventTypeCritical
		}

		ev := eventstore.Event{
			Component: componentName,
			Time:      now,
			Name:      eventName,
			Type:      string(eventType),
			Message:   inc.Message,
			ExtraInfo: map[string]string{
				EventKeyEntityID:  inc.EntityID,
				EventKeyErrorCode: inc.Error,
			},
		}
		if err := eventBucket.Insert(ctx, ev); err != nil {
			log.Logger.Warnw("failed to insert DCGM incident event", "component", componentName, "error", err)
		}
		prevKeys[key] = struct{}{} // suppress in-curr duplicates
	}
}

var healthCheckErrorCodeNames = map[dcgm.HealthCheckErrorCode]string{
	dcgm.DCGM_FR_OK:                              "DCGM_FR_OK",
	dcgm.DCGM_FR_UNKNOWN:                         "DCGM_FR_UNKNOWN",
	dcgm.DCGM_FR_UNRECOGNIZED:                    "DCGM_FR_UNRECOGNIZED",
	dcgm.DCGM_FR_PCI_REPLAY_RATE:                 "DCGM_FR_PCI_REPLAY_RATE",
	dcgm.DCGM_FR_VOLATILE_DBE_DETECTED:           "DCGM_FR_VOLATILE_DBE_DETECTED",
	dcgm.DCGM_FR_VOLATILE_SBE_DETECTED:           "DCGM_FR_VOLATILE_SBE_DETECTED",
	dcgm.DCGM_FR_VOLATILE_SBE_DETECTED_TS:        "DCGM_FR_VOLATILE_SBE_DETECTED_TS",
	dcgm.DCGM_FR_RETIRED_PAGES_LIMIT:             "DCGM_FR_RETIRED_PAGES_LIMIT",
	dcgm.DCGM_FR_RETIRED_PAGES_DBE_LIMIT:         "DCGM_FR_RETIRED_PAGES_DBE_LIMIT",
	dcgm.DCGM_FR_CORRUPT_INFOROM:                 "DCGM_FR_CORRUPT_INFOROM",
	dcgm.DCGM_FR_CLOCK_THROTTLE_THERMAL:          "DCGM_FR_CLOCK_THROTTLE_THERMAL",
	dcgm.DCGM_FR_POWER_UNREADABLE:                "DCGM_FR_POWER_UNREADABLE",
	dcgm.DCGM_FR_CLOCK_THROTTLE_POWER:            "DCGM_FR_CLOCK_THROTTLE_POWER",
	dcgm.DCGM_FR_NVLINK_ERROR_THRESHOLD:          "DCGM_FR_NVLINK_ERROR_THRESHOLD",
	dcgm.DCGM_FR_NVLINK_DOWN:                     "DCGM_FR_NVLINK_DOWN",
	dcgm.DCGM_FR_NVSWITCH_FATAL_ERROR:            "DCGM_FR_NVSWITCH_FATAL_ERROR",
	dcgm.DCGM_FR_NVSWITCH_NON_FATAL_ERROR:        "DCGM_FR_NVSWITCH_NON_FATAL_ERROR",
	dcgm.DCGM_FR_NVSWITCH_DOWN:                   "DCGM_FR_NVSWITCH_DOWN",
	dcgm.DCGM_FR_NO_ACCESS_TO_FILE:               "DCGM_FR_NO_ACCESS_TO_FILE",
	dcgm.DCGM_FR_NVML_API:                        "DCGM_FR_NVML_API",
	dcgm.DCGM_FR_DEVICE_COUNT_MISMATCH:           "DCGM_FR_DEVICE_COUNT_MISMATCH",
	dcgm.DCGM_FR_BAD_PARAMETER:                   "DCGM_FR_BAD_PARAMETER",
	dcgm.DCGM_FR_CANNOT_OPEN_LIB:                 "DCGM_FR_CANNOT_OPEN_LIB",
	dcgm.DCGM_FR_DENYLISTED_DRIVER:               "DCGM_FR_DENYLISTED_DRIVER",
	dcgm.DCGM_FR_NVML_LIB_BAD:                    "DCGM_FR_NVML_LIB_BAD",
	dcgm.DCGM_FR_GRAPHICS_PROCESSES:              "DCGM_FR_GRAPHICS_PROCESSES",
	dcgm.DCGM_FR_HOSTENGINE_CONN:                 "DCGM_FR_HOSTENGINE_CONN",
	dcgm.DCGM_FR_FIELD_QUERY:                     "DCGM_FR_FIELD_QUERY",
	dcgm.DCGM_FR_BAD_CUDA_ENV:                    "DCGM_FR_BAD_CUDA_ENV",
	dcgm.DCGM_FR_PERSISTENCE_MODE:                "DCGM_FR_PERSISTENCE_MODE",
	dcgm.DCGM_FR_LOW_BANDWIDTH:                   "DCGM_FR_LOW_BANDWIDTH",
	dcgm.DCGM_FR_HIGH_LATENCY:                    "DCGM_FR_HIGH_LATENCY",
	dcgm.DCGM_FR_CANNOT_GET_FIELD_TAG:            "DCGM_FR_CANNOT_GET_FIELD_TAG",
	dcgm.DCGM_FR_FIELD_VIOLATION:                 "DCGM_FR_FIELD_VIOLATION",
	dcgm.DCGM_FR_FIELD_THRESHOLD:                 "DCGM_FR_FIELD_THRESHOLD",
	dcgm.DCGM_FR_FIELD_VIOLATION_DBL:             "DCGM_FR_FIELD_VIOLATION_DBL",
	dcgm.DCGM_FR_FIELD_THRESHOLD_DBL:             "DCGM_FR_FIELD_THRESHOLD_DBL",
	dcgm.DCGM_FR_UNSUPPORTED_FIELD_TYPE:          "DCGM_FR_UNSUPPORTED_FIELD_TYPE",
	dcgm.DCGM_FR_FIELD_THRESHOLD_TS:              "DCGM_FR_FIELD_THRESHOLD_TS",
	dcgm.DCGM_FR_FIELD_THRESHOLD_TS_DBL:          "DCGM_FR_FIELD_THRESHOLD_TS_DBL",
	dcgm.DCGM_FR_THERMAL_VIOLATIONS:              "DCGM_FR_THERMAL_VIOLATIONS",
	dcgm.DCGM_FR_THERMAL_VIOLATIONS_TS:           "DCGM_FR_THERMAL_VIOLATIONS_TS",
	dcgm.DCGM_FR_TEMP_VIOLATION:                  "DCGM_FR_TEMP_VIOLATION",
	dcgm.DCGM_FR_THROTTLING_VIOLATION:            "DCGM_FR_THROTTLING_VIOLATION",
	dcgm.DCGM_FR_INTERNAL:                        "DCGM_FR_INTERNAL",
	dcgm.DCGM_FR_PCIE_GENERATION:                 "DCGM_FR_PCIE_GENERATION",
	dcgm.DCGM_FR_PCIE_WIDTH:                      "DCGM_FR_PCIE_WIDTH",
	dcgm.DCGM_FR_ABORTED:                         "DCGM_FR_ABORTED",
	dcgm.DCGM_FR_TEST_DISABLED:                   "DCGM_FR_TEST_DISABLED",
	dcgm.DCGM_FR_CANNOT_GET_STAT:                 "DCGM_FR_CANNOT_GET_STAT",
	dcgm.DCGM_FR_STRESS_LEVEL:                    "DCGM_FR_STRESS_LEVEL",
	dcgm.DCGM_FR_CUDA_API:                        "DCGM_FR_CUDA_API",
	dcgm.DCGM_FR_FAULTY_MEMORY:                   "DCGM_FR_FAULTY_MEMORY",
	dcgm.DCGM_FR_CANNOT_SET_WATCHES:              "DCGM_FR_CANNOT_SET_WATCHES",
	dcgm.DCGM_FR_CUDA_UNBOUND:                    "DCGM_FR_CUDA_UNBOUND",
	dcgm.DCGM_FR_ECC_DISABLED:                    "DCGM_FR_ECC_DISABLED",
	dcgm.DCGM_FR_MEMORY_ALLOC:                    "DCGM_FR_MEMORY_ALLOC",
	dcgm.DCGM_FR_CUDA_DBE:                        "DCGM_FR_CUDA_DBE",
	dcgm.DCGM_FR_MEMORY_MISMATCH:                 "DCGM_FR_MEMORY_MISMATCH",
	dcgm.DCGM_FR_CUDA_DEVICE:                     "DCGM_FR_CUDA_DEVICE",
	dcgm.DCGM_FR_ECC_UNSUPPORTED:                 "DCGM_FR_ECC_UNSUPPORTED",
	dcgm.DCGM_FR_ECC_PENDING:                     "DCGM_FR_ECC_PENDING",
	dcgm.DCGM_FR_MEMORY_BANDWIDTH:                "DCGM_FR_MEMORY_BANDWIDTH",
	dcgm.DCGM_FR_TARGET_POWER:                    "DCGM_FR_TARGET_POWER",
	dcgm.DCGM_FR_API_FAIL:                        "DCGM_FR_API_FAIL",
	dcgm.DCGM_FR_API_FAIL_GPU:                    "DCGM_FR_API_FAIL_GPU",
	dcgm.DCGM_FR_CUDA_CONTEXT:                    "DCGM_FR_CUDA_CONTEXT",
	dcgm.DCGM_FR_DCGM_API:                        "DCGM_FR_DCGM_API",
	dcgm.DCGM_FR_CONCURRENT_GPUS:                 "DCGM_FR_CONCURRENT_GPUS",
	dcgm.DCGM_FR_TOO_MANY_ERRORS:                 "DCGM_FR_TOO_MANY_ERRORS",
	dcgm.DCGM_FR_NVLINK_CRC_ERROR_THRESHOLD:      "DCGM_FR_NVLINK_CRC_ERROR_THRESHOLD",
	dcgm.DCGM_FR_NVLINK_ERROR_CRITICAL:           "DCGM_FR_NVLINK_ERROR_CRITICAL",
	dcgm.DCGM_FR_ENFORCED_POWER_LIMIT:            "DCGM_FR_ENFORCED_POWER_LIMIT",
	dcgm.DCGM_FR_MEMORY_ALLOC_HOST:               "DCGM_FR_MEMORY_ALLOC_HOST",
	dcgm.DCGM_FR_GPU_OP_MODE:                     "DCGM_FR_GPU_OP_MODE",
	dcgm.DCGM_FR_NO_MEMORY_CLOCKS:                "DCGM_FR_NO_MEMORY_CLOCKS",
	dcgm.DCGM_FR_NO_GRAPHICS_CLOCKS:              "DCGM_FR_NO_GRAPHICS_CLOCKS",
	dcgm.DCGM_FR_HAD_TO_RESTORE_STATE:            "DCGM_FR_HAD_TO_RESTORE_STATE",
	dcgm.DCGM_FR_L1TAG_UNSUPPORTED:               "DCGM_FR_L1TAG_UNSUPPORTED",
	dcgm.DCGM_FR_L1TAG_MISCOMPARE:                "DCGM_FR_L1TAG_MISCOMPARE",
	dcgm.DCGM_FR_ROW_REMAP_FAILURE:               "DCGM_FR_ROW_REMAP_FAILURE",
	dcgm.DCGM_FR_UNCONTAINED_ERROR:               "DCGM_FR_UNCONTAINED_ERROR",
	dcgm.DCGM_FR_EMPTY_GPU_LIST:                  "DCGM_FR_EMPTY_GPU_LIST",
	dcgm.DCGM_FR_DBE_PENDING_PAGE_RETIREMENTS:    "DCGM_FR_DBE_PENDING_PAGE_RETIREMENTS",
	dcgm.DCGM_FR_UNCORRECTABLE_ROW_REMAP:         "DCGM_FR_UNCORRECTABLE_ROW_REMAP",
	dcgm.DCGM_FR_PENDING_ROW_REMAP:               "DCGM_FR_PENDING_ROW_REMAP",
	dcgm.DCGM_FR_BROKEN_P2P_MEMORY_DEVICE:        "DCGM_FR_BROKEN_P2P_MEMORY_DEVICE",
	dcgm.DCGM_FR_BROKEN_P2P_WRITER_DEVICE:        "DCGM_FR_BROKEN_P2P_WRITER_DEVICE",
	dcgm.DCGM_FR_NVSWITCH_NVLINK_DOWN:            "DCGM_FR_NVSWITCH_NVLINK_DOWN",
	dcgm.DCGM_FR_EUD_BINARY_PERMISSIONS:          "DCGM_FR_EUD_BINARY_PERMISSIONS",
	dcgm.DCGM_FR_EUD_NON_ROOT_USER:               "DCGM_FR_EUD_NON_ROOT_USER",
	dcgm.DCGM_FR_EUD_SPAWN_FAILURE:               "DCGM_FR_EUD_SPAWN_FAILURE",
	dcgm.DCGM_FR_EUD_TIMEOUT:                     "DCGM_FR_EUD_TIMEOUT",
	dcgm.DCGM_FR_EUD_ZOMBIE:                      "DCGM_FR_EUD_ZOMBIE",
	dcgm.DCGM_FR_EUD_NON_ZERO_EXIT_CODE:          "DCGM_FR_EUD_NON_ZERO_EXIT_CODE",
	dcgm.DCGM_FR_EUD_TEST_FAILED:                 "DCGM_FR_EUD_TEST_FAILED",
	dcgm.DCGM_FR_FILE_CREATE_PERMISSIONS:         "DCGM_FR_FILE_CREATE_PERMISSIONS",
	dcgm.DCGM_FR_PAUSE_RESUME_FAILED:             "DCGM_FR_PAUSE_RESUME_FAILED",
	dcgm.DCGM_FR_PCIE_H_REPLAY_VIOLATION:         "DCGM_FR_PCIE_H_REPLAY_VIOLATION",
	dcgm.DCGM_FR_GPU_EXPECTED_NVLINKS_UP:         "DCGM_FR_GPU_EXPECTED_NVLINKS_UP",
	dcgm.DCGM_FR_NVSWITCH_EXPECTED_NVLINKS_UP:    "DCGM_FR_NVSWITCH_EXPECTED_NVLINKS_UP",
	dcgm.DCGM_FR_XID_ERROR:                       "DCGM_FR_XID_ERROR",
	dcgm.DCGM_FR_SBE_VIOLATION:                   "DCGM_FR_SBE_VIOLATION",
	dcgm.DCGM_FR_DBE_VIOLATION:                   "DCGM_FR_DBE_VIOLATION",
	dcgm.DCGM_FR_PCIE_REPLAY_VIOLATION:           "DCGM_FR_PCIE_REPLAY_VIOLATION",
	dcgm.DCGM_FR_SBE_THRESHOLD_VIOLATION:         "DCGM_FR_SBE_THRESHOLD_VIOLATION",
	dcgm.DCGM_FR_DBE_THRESHOLD_VIOLATION:         "DCGM_FR_DBE_THRESHOLD_VIOLATION",
	dcgm.DCGM_FR_PCIE_REPLAY_THRESHOLD_VIOLATION: "DCGM_FR_PCIE_REPLAY_THRESHOLD_VIOLATION",
	dcgm.DCGM_FR_CUDA_FM_NOT_INITIALIZED:         "DCGM_FR_CUDA_FM_NOT_INITIALIZED",
	dcgm.DCGM_FR_SXID_ERROR:                      "DCGM_FR_SXID_ERROR",
	dcgm.DCGM_FR_GFLOPS_THRESHOLD_VIOLATION:      "DCGM_FR_GFLOPS_THRESHOLD_VIOLATION",
	dcgm.DCGM_FR_NAN_VALUE:                       "DCGM_FR_NAN_VALUE",
	dcgm.DCGM_FR_FABRIC_MANAGER_TRAINING_ERROR:   "DCGM_FR_FABRIC_MANAGER_TRAINING_ERROR",
	dcgm.DCGM_FR_BROKEN_P2P_PCIE_MEMORY_DEVICE:   "DCGM_FR_BROKEN_P2P_PCIE_MEMORY_DEVICE",
	dcgm.DCGM_FR_BROKEN_P2P_PCIE_WRITER_DEVICE:   "DCGM_FR_BROKEN_P2P_PCIE_WRITER_DEVICE",
	dcgm.DCGM_FR_BROKEN_P2P_NVLINK_MEMORY_DEVICE: "DCGM_FR_BROKEN_P2P_NVLINK_MEMORY_DEVICE",
	dcgm.DCGM_FR_BROKEN_P2P_NVLINK_WRITER_DEVICE: "DCGM_FR_BROKEN_P2P_NVLINK_WRITER_DEVICE",
	dcgm.DCGM_FR_ERROR_SENTINEL:                  "DCGM_FR_ERROR_SENTINEL",
}

var healthSystemNames = map[dcgm.HealthSystem]string{
	dcgm.DCGM_HEALTH_WATCH_PCIE:              "DCGM_HEALTH_WATCH_PCIE",
	dcgm.DCGM_HEALTH_WATCH_NVLINK:            "DCGM_HEALTH_WATCH_NVLINK",
	dcgm.DCGM_HEALTH_WATCH_PMU:               "DCGM_HEALTH_WATCH_PMU",
	dcgm.DCGM_HEALTH_WATCH_MCU:               "DCGM_HEALTH_WATCH_MCU",
	dcgm.DCGM_HEALTH_WATCH_MEM:               "DCGM_HEALTH_WATCH_MEM",
	dcgm.DCGM_HEALTH_WATCH_SM:                "DCGM_HEALTH_WATCH_SM",
	dcgm.DCGM_HEALTH_WATCH_INFOROM:           "DCGM_HEALTH_WATCH_INFOROM",
	dcgm.DCGM_HEALTH_WATCH_THERMAL:           "DCGM_HEALTH_WATCH_THERMAL",
	dcgm.DCGM_HEALTH_WATCH_POWER:             "DCGM_HEALTH_WATCH_POWER",
	dcgm.DCGM_HEALTH_WATCH_DRIVER:            "DCGM_HEALTH_WATCH_DRIVER",
	dcgm.DCGM_HEALTH_WATCH_NVSWITCH_NONFATAL: "DCGM_HEALTH_WATCH_NVSWITCH_NONFATAL",
	dcgm.DCGM_HEALTH_WATCH_NVSWITCH_FATAL:    "DCGM_HEALTH_WATCH_NVSWITCH_FATAL",
	dcgm.DCGM_HEALTH_WATCH_ALL:               "DCGM_HEALTH_WATCH_ALL",
}

// EnrichedIncident represents a DCGM incident with entity ID mapped to UUID for better usability.
type EnrichedIncident struct {
	// GPU UUID mapped from entity ID
	UUID string `json:"uuid"`
	// entity identifier derived from DCGM entity type and entity ID
	EntityID string `json:"-"`
	// error message from the incident
	Message string `json:"message"`
	// DCGM_FR_* error code as integer (backend-compatible)
	ErrorCode dcgm.HealthCheckErrorCode `json:"code"`
	// DCGM_HEALTH_WATCH_* system as integer (backend-compatible)
	System dcgm.HealthSystem `json:"system"`
	// health result level as integer (backend-compatible)
	Health dcgm.HealthResult `json:"health"`
}

// EnrichIncidents transforms DCGM incidents by mapping entity IDs to UUIDs.
// Returns enriched incidents with UUIDs instead of entity IDs for better usability.
func EnrichIncidents(incidents []dcgm.Incident, deviceMapping map[uint]string) []EnrichedIncident {
	if len(incidents) == 0 {
		return nil
	}

	enriched := make([]EnrichedIncident, 0, len(incidents))
	for _, incident := range incidents {
		// Map entity ID to UUID
		uuid := deviceMapping[incident.EntityInfo.EntityId]
		if uuid == "" {
			// Fallback if UUID not found
			uuid = fmt.Sprintf("device-%d", incident.EntityInfo.EntityId)
		}

		enriched = append(enriched, EnrichedIncident{
			UUID:      uuid,
			EntityID:  dcgmEntityID(incident.EntityInfo),
			Message:   incident.Error.Message,
			ErrorCode: incident.Error.Code,
			System:    incident.System,
			Health:    incident.Health,
		})
	}

	return enriched
}

// EnrichSwitchIncidents transforms NVSwitch incidents using switch-specific identifiers.
func EnrichSwitchIncidents(incidents []dcgm.Incident) []EnrichedIncident {
	if len(incidents) == 0 {
		return nil
	}

	enriched := make([]EnrichedIncident, 0, len(incidents))
	for _, incident := range incidents {
		enriched = append(enriched, EnrichedIncident{
			UUID:      fmt.Sprintf("nvswitch-%d", incident.EntityInfo.EntityId),
			EntityID:  dcgmEntityID(incident.EntityInfo),
			Message:   incident.Error.Message,
			ErrorCode: incident.Error.Code,
			System:    incident.System,
			Health:    incident.Health,
		})
	}

	return enriched
}

func (e EnrichedIncident) ToHealthStateIncident() apiv1.HealthStateIncident {
	return apiv1.HealthStateIncident{
		EntityID: e.EntityID,
		Message:  e.Message,
		Severity: healthResultToSeverity(e.Health),
		Error:    healthCheckErrorCodeString(e.ErrorCode),
	}
}

func dcgmEntityID(entity dcgm.GroupEntityPair) string {
	return fmt.Sprintf("%s-%d", entity.EntityGroupId.String(), entity.EntityId)
}

func ToHealthStateIncidents(incidents []EnrichedIncident) []apiv1.HealthStateIncident {
	if len(incidents) == 0 {
		return nil
	}

	result := make([]apiv1.HealthStateIncident, 0, len(incidents))
	for _, incident := range incidents {
		result = append(result, incident.ToHealthStateIncident())
	}
	return result
}

// FormatIncidents formats enriched DCGM incidents into a human-readable string.
func FormatIncidents(prefix string, incidents []EnrichedIncident) string {
	if len(incidents) == 0 {
		return prefix
	}

	devices := make(map[string]struct{})
	for _, incident := range incidents {
		if incident.UUID != "" {
			devices[incident.UUID] = struct{}{}
		}
	}

	return fmt.Sprintf("%s: %d incident(s) across %d device(s)",
		prefix,
		len(incidents),
		len(devices),
	)
}

func healthCheckErrorCodeString(code dcgm.HealthCheckErrorCode) string {
	// Some DCGM codes have multiple macro aliases for the same numeric value.
	// We return the first canonical name defined in the pinned go-dcgm constants.
	if name, ok := healthCheckErrorCodeNames[code]; ok {
		return name
	}
	return fmt.Sprintf("DCGM_FR_UNKNOWN(%d)", code)
}

func healthSystemString(system dcgm.HealthSystem) string {
	if name, ok := healthSystemNames[system]; ok {
		return name
	}
	return fmt.Sprintf("DCGM_HEALTH_WATCH_UNKNOWN(0x%X)", uint(system))
}

func healthResultToSeverity(result dcgm.HealthResult) apiv1.HealthStateType {
	switch result {
	case dcgm.DCGM_HEALTH_RESULT_WARN:
		return apiv1.HealthStateTypeDegraded
	case dcgm.DCGM_HEALTH_RESULT_FAIL:
		return apiv1.HealthStateTypeUnhealthy
	default:
		return ""
	}
}
