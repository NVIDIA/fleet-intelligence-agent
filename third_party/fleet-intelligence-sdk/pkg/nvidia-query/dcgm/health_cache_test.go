// SPDX-FileCopyrightText: Copyright (c) 2024, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dcgm

import (
	"context"
	"errors"
	"testing"
	"time"

	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
)

func TestHealthCheckDirectGroupHandleNotReadyIsTransient(t *testing.T) {
	inst := &healthCacheMockInstance{
		dcgmExists:   true,
		groupHandle:  dcgm.GroupHandle{},
		watchedField: []dcgm.Short{},
	}

	_, err := healthCheckDirect(inst)
	if err == nil {
		t.Fatalf("expected error for uninitialized group handle")
	}
	if !errors.Is(err, errTransientGroupNotReady) {
		t.Fatalf("expected transient group-not-ready error, got: %v", err)
	}
}

func TestHealthCachePollSkipsUninitializedGroupHandle(t *testing.T) {
	hc := NewHealthCache(
		context.Background(),
		&healthCacheMockInstance{
			dcgmExists:   true,
			groupHandle:  dcgm.GroupHandle{},
			watchedField: []dcgm.Short{},
		},
		time.Second,
	)

	if err := hc.Poll(); err != nil {
		t.Fatalf("Poll() should treat uninitialized group handle as transient, got: %v", err)
	}

	_, _, err := hc.GetResult(dcgm.DCGM_HEALTH_WATCH_PCIE)
	if err != nil {
		t.Fatalf("GetResult() should not expose transient group-not-ready as lastError, got: %v", err)
	}
}

type healthCacheMockInstance struct {
	dcgmExists   bool
	groupHandle  dcgm.GroupHandle
	watchedField []dcgm.Short
}

func (m *healthCacheMockInstance) DCGMExists() bool { return m.dcgmExists }

func (m *healthCacheMockInstance) AddEntityToGroup(entityID uint) error { return nil }

func (m *healthCacheMockInstance) AddHealthWatch(system dcgm.HealthSystem) error { return nil }

func (m *healthCacheMockInstance) RemoveHealthWatch(system dcgm.HealthSystem) error { return nil }

func (m *healthCacheMockInstance) HealthCheck(system dcgm.HealthSystem) (dcgm.HealthResult, []dcgm.Incident, error) {
	return dcgm.DCGM_HEALTH_RESULT_PASS, nil, nil
}

func (m *healthCacheMockInstance) AddFieldsToWatch(fields []dcgm.Short) error {
	m.watchedField = append(m.watchedField, fields...)
	return nil
}

func (m *healthCacheMockInstance) GetWatchedFields() []dcgm.Short {
	fieldsCopy := make([]dcgm.Short, len(m.watchedField))
	copy(fieldsCopy, m.watchedField)
	return fieldsCopy
}

func (m *healthCacheMockInstance) RemoveFieldsFromWatch(fields []dcgm.Short) error { return nil }

func (m *healthCacheMockInstance) GetLatestValuesForFields(deviceID uint, fields []dcgm.Short) ([]dcgm.FieldValue_v1, error) {
	return nil, nil
}

func (m *healthCacheMockInstance) GetGroupHandle() dcgm.GroupHandle { return m.groupHandle }

func (m *healthCacheMockInstance) GetDevices() []DeviceInfo { return nil }

func (m *healthCacheMockInstance) Shutdown() error { return nil }
