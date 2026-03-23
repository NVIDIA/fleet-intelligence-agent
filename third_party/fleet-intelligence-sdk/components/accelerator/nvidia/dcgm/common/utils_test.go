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

package common

import (
	"context"
	"testing"
	"time"

	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/eventstore"
)

func TestFormatEnrichedIncidents(t *testing.T) {
	tests := []struct {
		name      string
		prefix    string
		incidents []EnrichedIncident
		want      string
	}{
		{
			name:      "no incidents",
			prefix:    "test prefix",
			incidents: nil,
			want:      "test prefix",
		},
		{
			name:      "empty incidents",
			prefix:    "test prefix",
			incidents: []EnrichedIncident{},
			want:      "test prefix",
		},
		{
			name:   "single incident",
			prefix: "thermal warning",
			incidents: []EnrichedIncident{
				{
					UUID:      "GPU-46a3bbe2-3e87-3dde-b464-a03eba0c21d7",
					Message:   "Temperature above threshold",
					ErrorCode: "DCGM_FR_TEMP_VIOLATION",
				},
			},
			want: "thermal warning: 1 incident(s) across 1 device(s)",
		},
		{
			name:   "multiple incidents",
			prefix: "memory failure",
			incidents: []EnrichedIncident{
				{
					UUID:      "GPU-46a3bbe2-3e87-3dde-b464-a03eba0c21d7",
					Message:   "DBE detected",
					ErrorCode: "DCGM_FR_VOLATILE_DBE_DETECTED",
				},
				{
					UUID:      "GPU-7b4f2c1a-8d6e-4c5b-9a1f-2e3d4c5a6b7c",
					Message:   "Row remap failure",
					ErrorCode: "DCGM_FR_ROW_REMAP_FAILURE",
				},
			},
			want: "memory failure: 2 incident(s) across 2 device(s)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatIncidents(tt.prefix, tt.incidents)
			if got != tt.want {
				t.Errorf("FormatIncidents() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestToHealthStateIncidents(t *testing.T) {
	got := ToHealthStateIncidents([]EnrichedIncident{
		{
			UUID:      "GPU-1234",
			EntityID:  "GPU-0",
			Message:   "Power violation",
			ErrorCode: "DCGM_FR_POWER_VIOLATION",
			Severity:  apiv1.HealthStateTypeUnhealthy,
		},
	})

	if len(got) != 1 {
		t.Fatalf("len(ToHealthStateIncidents()) = %d, want 1", len(got))
	}

	want := apiv1.HealthStateIncident{
		EntityID: "GPU-0",
		Message:  "Power violation",
		Severity: apiv1.HealthStateTypeUnhealthy,
		Error:    "DCGM_FR_POWER_VIOLATION",
	}
	if got[0] != want {
		t.Fatalf("ToHealthStateIncidents()[0] = %#v, want %#v", got[0], want)
	}
}

func TestHealthCheckErrorCodeString(t *testing.T) {
	if got := healthCheckErrorCodeString(dcgm.DCGM_FR_VOLATILE_DBE_DETECTED); got != "DCGM_FR_VOLATILE_DBE_DETECTED" {
		t.Fatalf("healthCheckErrorCodeString(known) = %q", got)
	}
	if got := healthCheckErrorCodeString(dcgm.DCGM_FR_XID_ERROR); got != "DCGM_FR_XID_ERROR" {
		t.Fatalf("healthCheckErrorCodeString(late known) = %q", got)
	}
	if got := healthCheckErrorCodeString(dcgm.DCGM_FR_ERROR_SENTINEL); got != "DCGM_FR_ERROR_SENTINEL" {
		t.Fatalf("healthCheckErrorCodeString(sentinel) = %q", got)
	}
	if got := healthCheckErrorCodeString(dcgm.HealthCheckErrorCode(9999)); got != "DCGM_FR_UNKNOWN(9999)" {
		t.Fatalf("healthCheckErrorCodeString(unknown) = %q", got)
	}
}

func TestHealthSystemString(t *testing.T) {
	if got := healthSystemString(dcgm.DCGM_HEALTH_WATCH_MEM); got != "DCGM_HEALTH_WATCH_MEM" {
		t.Fatalf("healthSystemString(single) = %q", got)
	}
	if got := healthSystemString(dcgm.DCGM_HEALTH_WATCH_DRIVER); got != "DCGM_HEALTH_WATCH_DRIVER" {
		t.Fatalf("healthSystemString(driver) = %q", got)
	}
	if got := healthSystemString(dcgm.DCGM_HEALTH_WATCH_NVSWITCH_FATAL); got != "DCGM_HEALTH_WATCH_NVSWITCH_FATAL" {
		t.Fatalf("healthSystemString(nvswitch fatal) = %q", got)
	}
	if got := healthSystemString(dcgm.DCGM_HEALTH_WATCH_ALL); got != "DCGM_HEALTH_WATCH_ALL" {
		t.Fatalf("healthSystemString(all) = %q", got)
	}
	if got := healthSystemString(dcgm.DCGM_HEALTH_WATCH_POWER | dcgm.DCGM_HEALTH_WATCH_THERMAL); got != "DCGM_HEALTH_WATCH_UNKNOWN(0x180)" {
		t.Fatalf("healthSystemString(mask) = %q", got)
	}
	if got := healthSystemString(dcgm.DCGM_HEALTH_WATCH_NVLINK | dcgm.DCGM_HEALTH_WATCH_DRIVER | dcgm.DCGM_HEALTH_WATCH_NVSWITCH_FATAL); got != "DCGM_HEALTH_WATCH_UNKNOWN(0xA02)" {
		t.Fatalf("healthSystemString(composite mask) = %q", got)
	}
}

func TestHealthResultToSeverity(t *testing.T) {
	if got := healthResultToSeverity(dcgm.DCGM_HEALTH_RESULT_WARN); got != apiv1.HealthStateTypeDegraded {
		t.Fatalf("healthResultToSeverity(WARN) = %q", got)
	}
	if got := healthResultToSeverity(dcgm.DCGM_HEALTH_RESULT_FAIL); got != apiv1.HealthStateTypeUnhealthy {
		t.Fatalf("healthResultToSeverity(FAIL) = %q", got)
	}
}

// fakeEventBucket is a minimal in-memory eventstore.Bucket for testing.
type fakeEventBucket struct {
	inserted []eventstore.Event
}

func (f *fakeEventBucket) Name() string { return "fake" }
func (f *fakeEventBucket) Insert(_ context.Context, ev eventstore.Event) error {
	f.inserted = append(f.inserted, ev)
	return nil
}
func (f *fakeEventBucket) Find(_ context.Context, _ eventstore.Event) (*eventstore.Event, error) {
	return nil, nil
}
func (f *fakeEventBucket) Get(_ context.Context, _ time.Time) (eventstore.Events, error) {
	return nil, nil
}
func (f *fakeEventBucket) Latest(_ context.Context) (*eventstore.Event, error) { return nil, nil }
func (f *fakeEventBucket) Purge(_ context.Context, _ int64) (int, error)       { return 0, nil }
func (f *fakeEventBucket) Close()                                               {}

func TestEmitNewIncidentEvents(t *testing.T) {
	now := time.Now().UTC()
	ctx := context.Background()

	gpu0Thermal := EnrichedIncident{
		UUID:      "GPU-0000",
		ErrorCode: "DCGM_FR_TEMP_VIOLATION",
		System:    "DCGM_HEALTH_WATCH_THERMAL",
		Message:   "temp too high",
		Severity:  apiv1.HealthStateTypeDegraded,
	}
	gpu1Mem := EnrichedIncident{
		UUID:      "GPU-0001",
		ErrorCode: "DCGM_FR_VOLATILE_DBE_DETECTED",
		System:    "DCGM_HEALTH_WATCH_MEM",
		Message:   "memory error",
		Severity:  apiv1.HealthStateTypeUnhealthy,
	}

	tests := []struct {
		name          string
		prev          []EnrichedIncident
		curr          []EnrichedIncident
		wantCount     int
		wantEventType string // of first event, if any
	}{
		{
			name:      "no incidents",
			prev:      nil,
			curr:      nil,
			wantCount: 0,
		},
		{
			name:      "same incident in prev and curr — no new event",
			prev:      []EnrichedIncident{gpu0Thermal},
			curr:      []EnrichedIncident{gpu0Thermal},
			wantCount: 0,
		},
		{
			name:          "new incident not in prev — event emitted",
			prev:          nil,
			curr:          []EnrichedIncident{gpu0Thermal},
			wantCount:     1,
			wantEventType: string(apiv1.EventTypeWarning),
		},
		{
			name:          "unhealthy severity maps to critical",
			prev:          nil,
			curr:          []EnrichedIncident{gpu1Mem},
			wantCount:     1,
			wantEventType: string(apiv1.EventTypeCritical),
		},
		{
			name:      "multiple new incidents all emitted",
			prev:      nil,
			curr:      []EnrichedIncident{gpu0Thermal, gpu1Mem},
			wantCount: 2,
		},
		{
			name:      "one existing one new — only new emitted",
			prev:      []EnrichedIncident{gpu0Thermal},
			curr:      []EnrichedIncident{gpu0Thermal, gpu1Mem},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket := &fakeEventBucket{}
			EmitNewIncidentEvents(ctx, now, "test-component", "test_incident", bucket, tt.prev, tt.curr)

			if len(bucket.inserted) != tt.wantCount {
				t.Fatalf("inserted %d events, want %d", len(bucket.inserted), tt.wantCount)
			}
			if tt.wantCount > 0 {
				ev := bucket.inserted[0]
				if ev.Name != "test_incident" {
					t.Errorf("event Name = %q, want %q", ev.Name, "test_incident")
				}
				if tt.wantEventType != "" && ev.Type != tt.wantEventType {
					t.Errorf("event Type = %q, want %q", ev.Type, tt.wantEventType)
				}
				if ev.ExtraInfo[EventKeyUUID] == "" {
					t.Errorf("event ExtraInfo[uuid] is empty")
				}
				if ev.ExtraInfo[EventKeyErrorCode] == "" {
					t.Errorf("event ExtraInfo[error_code] is empty")
				}
				if ev.ExtraInfo[EventKeySystem] == "" {
					t.Errorf("event ExtraInfo[system] is empty")
				}
			}
		})
	}
}

func TestEnrichSwitchIncidents_UsesSwitchIdentifiers(t *testing.T) {
	incidents := []dcgm.Incident{
		{
			EntityInfo: dcgm.GroupEntityPair{
				EntityGroupId: dcgm.FE_SWITCH,
				EntityId:      7,
			},
			Error: dcgm.DiagErrorDetail{
				Message: "switch failure",
				Code:    dcgm.DCGM_FR_NVSWITCH_FATAL_ERROR,
			},
			System: dcgm.DCGM_HEALTH_WATCH_NVSWITCH_FATAL,
			Health: dcgm.DCGM_HEALTH_RESULT_FAIL,
		},
	}

	got := EnrichSwitchIncidents(incidents)
	if len(got) != 1 {
		t.Fatalf("len(EnrichSwitchIncidents()) = %d, want 1", len(got))
	}
	if got[0].UUID != "nvswitch-7" {
		t.Fatalf("EnrichSwitchIncidents()[0].UUID = %q, want nvswitch-7", got[0].UUID)
	}
}
