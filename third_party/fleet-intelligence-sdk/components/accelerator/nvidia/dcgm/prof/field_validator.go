package prof

import (
	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
)

// fieldValidator checks if profiling fields are supported by the GPU hardware
type fieldValidator struct {
	dcpSupported    bool // Whether DCP is supported at all
	supportedFields map[uint]bool
}

// newFieldValidator creates validator for a specific GPU device
// Returns partial validator even on errors (graceful degradation)
func newFieldValidator(deviceID uint) *fieldValidator {
	v := &fieldValidator{
		supportedFields: make(map[uint]bool),
		dcpSupported:    false,
	}

	// Query hardware-supported metric groups
	// This checks if the GPU hardware supports profiling metrics
	groups, err := dcgm.GetSupportedMetricGroups(deviceID)
	if err != nil {
		// EXPECTED on:
		// - Consumer GPUs (RTX series)
		// - When profiling module not loaded
		// - When hostengine doesn't support profiling
		log.Logger.Infow("DCP metrics not available on device",
			"deviceID", deviceID,
			"error", err,
			"impact", "profiling metrics disabled")
		return v // Return validator with dcpSupported=false
	}

	v.dcpSupported = true
	for _, group := range groups {
		for _, fieldID := range group.FieldIds {
			v.supportedFields[fieldID] = true
		}
	}

	log.Logger.Infow("DCP support detected",
		"deviceID", deviceID,
		"supportedFields", len(v.supportedFields))

	return v
}

// validateFields filters profiling fields based on hardware support
// Returns only fields that are supported by the GPU hardware
func (v *fieldValidator) validateFields(fields []dcgm.Short) []dcgm.Short {
	// If DCP is not supported at all, return empty list
	if !v.dcpSupported {
		return []dcgm.Short{}
	}

	// Filter fields to only those supported by hardware
	var validFields []dcgm.Short
	for _, field := range fields {
		if v.supportedFields[uint(field)] {
			validFields = append(validFields, field)
		}
	}

	return validFields
}
