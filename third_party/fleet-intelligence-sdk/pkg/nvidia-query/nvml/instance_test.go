package nvml

import (
	"errors"
	"testing"

	nvmllib "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/nvml/lib"
)

func TestInstanceV2(t *testing.T) {
	inst, err := New()
	if errors.Is(err, nvmllib.ErrNVMLNotFound) {
		t.Skipf("nvml not installed, skipping")
	}
	if err != nil {
		t.Fatalf("failed to create instance: %v", err)
	}
	t.Logf("instance mem cap %+v", inst.GetMemoryErrorManagementCapabilities())
}
