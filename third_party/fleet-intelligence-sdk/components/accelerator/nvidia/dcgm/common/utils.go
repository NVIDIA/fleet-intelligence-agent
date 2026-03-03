// Package common provides common utilities for DCGM health monitoring components.
package common

import (
	"fmt"
	"strings"

	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
)

// EnrichedIncident represents a DCGM incident with entity ID mapped to UUID for better usability.
type EnrichedIncident struct {
	// GPU UUID mapped from entity ID
	UUID string `json:"uuid"`
	// error message from the incident
	Message string `json:"message"`
	// error code from the incident
	Code interface{} `json:"code"`
	// health system that reported the incident
	System dcgm.HealthSystem `json:"system"`
	// health result level (PASS/WARN/FAIL)
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
			UUID:    uuid,
			Message: incident.Error.Message,
			Code:    incident.Error.Code,
			System:  incident.System,
			Health:  incident.Health,
		})
	}

	return enriched
}

// FormatIncidents formats enriched DCGM incidents into a human-readable string.
// Formats enriched DCGM incidents with UUIDs into a human-readable string.
func FormatIncidents(prefix string, incidents []EnrichedIncident) string {
	if len(incidents) == 0 {
		return prefix
	}

	var parts []string
	for _, incident := range incidents {
		msg := fmt.Sprintf("GPU %s: %s (code: %v)",
			incident.UUID,
			incident.Message,
			incident.Code)
		parts = append(parts, msg)
	}

	return fmt.Sprintf("%s - %s", prefix, strings.Join(parts, "; "))
}
