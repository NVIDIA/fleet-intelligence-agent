package prof

import (
	"testing"

	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
)

func TestFieldValidator_NoDCPSupport(t *testing.T) {
	// Simulate validator when DCP is not supported
	v := &fieldValidator{
		dcpSupported:    false,
		supportedFields: make(map[uint]bool),
	}

	fields := []dcgm.Short{
		dcgm.DCGM_FI_PROF_GR_ENGINE_ACTIVE,
		dcgm.DCGM_FI_PROF_SM_ACTIVE,
		dcgm.DCGM_FI_PROF_DRAM_ACTIVE,
	}

	validFields := v.validateFields(fields)

	// All fields should fail validation when DCP is not supported
	if len(validFields) != 0 {
		t.Errorf("Expected 0 valid fields when DCP not supported, got %d", len(validFields))
	}
}

func TestFieldValidator_WithDCPSupport(t *testing.T) {
	// Simulate validator when DCP is supported
	v := &fieldValidator{
		dcpSupported: true,
		supportedFields: map[uint]bool{
			uint(dcgm.DCGM_FI_PROF_GR_ENGINE_ACTIVE): true,
			uint(dcgm.DCGM_FI_PROF_SM_ACTIVE):        true,
			// DCGM_FI_PROF_DRAM_ACTIVE is intentionally missing
		},
	}

	fields := []dcgm.Short{
		dcgm.DCGM_FI_PROF_GR_ENGINE_ACTIVE, // supported
		dcgm.DCGM_FI_PROF_SM_ACTIVE,        // supported
		dcgm.DCGM_FI_PROF_DRAM_ACTIVE,      // not supported
	}

	validFields := v.validateFields(fields)

	// Should have 2 valid fields (only the supported ones)
	if len(validFields) != 2 {
		t.Errorf("Expected 2 valid fields, got %d", len(validFields))
	}

	// Verify the correct fields passed validation
	expectedFields := map[dcgm.Short]bool{
		dcgm.DCGM_FI_PROF_GR_ENGINE_ACTIVE: true,
		dcgm.DCGM_FI_PROF_SM_ACTIVE:        true,
	}

	for _, field := range validFields {
		if !expectedFields[field] {
			t.Errorf("Unexpected field in validFields: %d", field)
		}
	}
}

func TestFieldValidator_AllFieldsSupported(t *testing.T) {
	// Simulate validator when all fields are supported
	v := &fieldValidator{
		dcpSupported: true,
		supportedFields: map[uint]bool{
			uint(dcgm.DCGM_FI_PROF_GR_ENGINE_ACTIVE): true,
			uint(dcgm.DCGM_FI_PROF_SM_ACTIVE):        true,
			uint(dcgm.DCGM_FI_PROF_DRAM_ACTIVE):      true,
		},
	}

	fields := []dcgm.Short{
		dcgm.DCGM_FI_PROF_GR_ENGINE_ACTIVE,
		dcgm.DCGM_FI_PROF_SM_ACTIVE,
		dcgm.DCGM_FI_PROF_DRAM_ACTIVE,
	}

	validFields := v.validateFields(fields)

	// All fields should pass validation
	if len(validFields) != len(fields) {
		t.Errorf("Expected %d valid fields, got %d", len(fields), len(validFields))
	}
}

func TestFieldValidator_EmptyFieldList(t *testing.T) {
	// Validator with DCP support
	v := &fieldValidator{
		dcpSupported: true,
		supportedFields: map[uint]bool{
			uint(dcgm.DCGM_FI_PROF_GR_ENGINE_ACTIVE): true,
		},
	}

	fields := []dcgm.Short{}

	validFields := v.validateFields(fields)

	// Should have no valid fields for empty input
	if len(validFields) != 0 {
		t.Errorf("Expected 0 valid fields for empty input, got %d", len(validFields))
	}
}
