package testutil

import (
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/nvml/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
)

// CreatePersistenceModeDevice creates a new mock device for persistence mode testing
func CreatePersistenceModeDevice(
	uuid string,
	persistenceMode nvml.EnableState,
	persistenceModeRet nvml.Return,
) device.Device {
	mockDevice := &mock.Device{
		GetPersistenceModeFunc: func() (nvml.EnableState, nvml.Return) {
			return persistenceMode, persistenceModeRet
		},
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
	}

	return NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")
}
