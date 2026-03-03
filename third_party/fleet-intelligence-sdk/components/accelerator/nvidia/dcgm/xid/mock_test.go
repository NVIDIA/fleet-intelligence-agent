package xid

import (
	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
	nvidiadcgm "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/dcgm"
)

// mockDCGMInstance is a minimal mock for testing xid component behavior
type mockDCGMInstance struct {
	dcgmExists bool
}

func (m *mockDCGMInstance) DCGMExists() bool {
	return m.dcgmExists
}

func (m *mockDCGMInstance) AddEntityToGroup(entityID uint) error {
	return nil
}

func (m *mockDCGMInstance) AddHealthWatch(system dcgm.HealthSystem) error {
	return nil
}

func (m *mockDCGMInstance) RemoveHealthWatch(system dcgm.HealthSystem) error {
	return nil
}

func (m *mockDCGMInstance) HealthCheck(system dcgm.HealthSystem) (dcgm.HealthResult, []dcgm.Incident, error) {
	var result dcgm.HealthResult
	return result, nil, nil
}

func (m *mockDCGMInstance) AddFieldsToWatch(fields []dcgm.Short) error {
	return nil
}

func (m *mockDCGMInstance) GetWatchedFields() []dcgm.Short {
	return nil
}

func (m *mockDCGMInstance) RemoveFieldsFromWatch(fields []dcgm.Short) error {
	return nil
}

func (m *mockDCGMInstance) GetLatestValuesForFields(deviceID uint, fields []dcgm.Short) ([]dcgm.FieldValue_v1, error) {
	return nil, nil
}

func (m *mockDCGMInstance) GetGroupHandle() dcgm.GroupHandle {
	return dcgm.GroupHandle{}
}

func (m *mockDCGMInstance) GetDevices() []nvidiadcgm.DeviceInfo {
	return []nvidiadcgm.DeviceInfo{{ID: 0}}
}

func (m *mockDCGMInstance) GetExistingPolicies() *dcgm.PolicyStatus {
	return nil
}

func (m *mockDCGMInstance) Shutdown() error {
	return nil
}
