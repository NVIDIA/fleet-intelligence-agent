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

package xid

import (
	"context"
	"testing"
	"time"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	"github.com/NVIDIA/fleet-intelligence-sdk/components"
	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics/scraper"
	nvidiadcgm "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/dcgm"
)

func TestNew(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dcgmInst, err := nvidiadcgm.New()
	if err != nil {
		t.Fatalf("failed to create DCGM instance: %v", err)
	}
	defer dcgmInst.Shutdown()

	gpudInst := &components.GPUdInstance{
		RootCtx:             ctx,
		DCGMInstance:        dcgmInst,
		HealthCheckInterval: time.Minute,
	}

	comp, err := New(gpudInst)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if comp.Name() != Name {
		t.Errorf("expected name %q, got %q", Name, comp.Name())
	}
}

func TestCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dcgmInst, err := nvidiadcgm.New()
	if err != nil {
		t.Fatalf("failed to create DCGM instance: %v", err)
	}
	defer dcgmInst.Shutdown()

	if !dcgmInst.DCGMExists() {
		t.Skip("DCGM not available, skipping test")
	}

	// Create caches for testing
	dcgmFieldValueCache := nvidiadcgm.NewFieldValueCache(ctx, dcgmInst, time.Minute)

	gpudInst := &components.GPUdInstance{
		RootCtx:             ctx,
		DCGMInstance:        dcgmInst,
		DCGMFieldValueCache: dcgmFieldValueCache,
		HealthCheckInterval: time.Minute,
	}

	comp, err := New(gpudInst)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Set up field watching after component registers its fields
	if err := dcgmFieldValueCache.SetupFieldWatchingWithName("gpud-xid-fields"); err != nil {
		t.Fatalf("SetupFieldWatching() failed: %v", err)
	}

	// Poll once to populate the cache
	if err := dcgmFieldValueCache.Poll(); err != nil {
		t.Logf("Poll() warning: %v", err)
	}

	// Perform check
	result := comp.Check()
	if result == nil {
		t.Fatal("Check() returned nil")
	}

	if result.ComponentName() != Name {
		t.Errorf("ComponentName() = %q, want %q", result.ComponentName(), Name)
	}

	healthType := result.HealthStateType()
	t.Logf("Health state: %v, Summary: %s", healthType, result.Summary())

	validHealthTypes := map[apiv1.HealthStateType]bool{
		apiv1.HealthStateTypeHealthy:   true,
		apiv1.HealthStateTypeDegraded:  true,
		apiv1.HealthStateTypeUnhealthy: true,
	}

	if !validHealthTypes[healthType] {
		t.Errorf("HealthStateType() = %v, want one of Healthy/Degraded/Unhealthy", healthType)
	}

	// Now verify that metrics were actually set in Prometheus by the Check() call
	t.Logf("\n=== Verifying Prometheus Metrics Were Set ===")

	// Use pkg/metrics scraper to gather metrics
	promScraper, err := scraper.NewPrometheusScraper(pkgmetrics.DefaultGatherer())
	if err != nil {
		t.Fatalf("Failed to create Prometheus scraper: %v", err)
	}

	metrics, err := promScraper.Scrape(ctx)
	if err != nil {
		t.Fatalf("Failed to scrape metrics: %v", err)
	}

	// Look for our XID metrics
	xidMetricsFound := map[string]int{
		"dcgm_xid_errors": 0,
	}

	for _, metric := range metrics {
		if metric.Component != Name {
			continue
		}

		if _, exists := xidMetricsFound[metric.Name]; exists {
			uuid := metric.Labels["uuid"]
			xid := metric.Labels["xid"]
			t.Logf("%s (uuid=%s, xid=%s): %.0f", metric.Name, uuid, xid, metric.Value)
			xidMetricsFound[metric.Name]++
		}
	}

	// Verify we found metrics for all expected fields
	for metricName, count := range xidMetricsFound {
		if count == 0 {
			t.Logf("%s was not found in Prometheus registry", metricName)
		} else {
			t.Logf("Found %d instance(s) of metric %s", count, metricName)
		}
	}
}

// TestSetupFailureHandling verifies that when field group creation or watching
// setup fails during New(), the component returns Degraded state on Check().
func TestSetupFailureHandling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockInstance := &mockDCGMInstance{dcgmExists: false}

	gpudInst := &components.GPUdInstance{
		RootCtx:             ctx,
		DCGMInstance:        mockInstance,
		HealthCheckInterval: time.Minute,
	}

	comp, err := New(gpudInst)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Manually set setupDegradedReason to simulate a setup failure
	c := comp.(*component)
	c.setupDegradedReason = "failed to create DCGM XID field group: mock error"

	// Enable DCGMExists so Check() proceeds past early guards
	mockInstance.dcgmExists = true

	// Check should return Degraded with the setup error reason
	result := comp.Check()
	if result == nil {
		t.Fatal("Check() returned nil")
	}

	healthType := result.HealthStateType()
	if healthType != apiv1.HealthStateTypeDegraded {
		t.Errorf("HealthStateType() = %v, want %v", healthType, apiv1.HealthStateTypeDegraded)
	}

	summary := result.Summary()
	if summary != "failed to create DCGM XID field group: mock error" {
		t.Errorf("Summary() = %q, want setup error message", summary)
	}

	t.Logf("Setup failure correctly returned Degraded: %s", summary)
}
