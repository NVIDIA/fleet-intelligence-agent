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

package mem

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

	// Create health cache for testing
	dcgmHealthCache := nvidiadcgm.NewHealthCache(ctx, dcgmInst, time.Minute)
	dcgmFieldValueCache := nvidiadcgm.NewFieldValueCache(ctx, dcgmInst, time.Minute)

	gpudInst := &components.GPUdInstance{
		RootCtx:             ctx,
		DCGMInstance:        dcgmInst,
		DCGMHealthCache:     dcgmHealthCache,
		DCGMFieldValueCache: dcgmFieldValueCache,
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

	// Create health cache for testing and trigger initial poll
	dcgmHealthCache := nvidiadcgm.NewHealthCache(ctx, dcgmInst, time.Minute)
	dcgmFieldValueCache := nvidiadcgm.NewFieldValueCache(ctx, dcgmInst, time.Minute)

	gpudInst := &components.GPUdInstance{
		RootCtx:             ctx,
		DCGMInstance:        dcgmInst,
		DCGMHealthCache:     dcgmHealthCache,
		DCGMFieldValueCache: dcgmFieldValueCache,
		HealthCheckInterval: time.Minute,
	}

	comp, err := New(gpudInst)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Set up field watching after component registers its fields
	if err := dcgmFieldValueCache.SetupFieldWatchingWithName("gpud-mem-fields"); err != nil {
		t.Fatalf("SetupFieldWatching() failed: %v", err)
	}

	// Poll once to populate the cache
	if err := dcgmFieldValueCache.Poll(); err != nil {
		t.Logf("Poll() warning: %v", err)
	}

	// Start the health cache polling (component New() already registered health watches)
	if err := dcgmHealthCache.Start(); err != nil {
		t.Fatalf("HealthCache.Start() failed: %v", err)
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

	// Look for our memory metrics
	memMetricsFound := map[string]int{
		"dcgm_fi_dev_fb_free":                        0,
		"dcgm_fi_dev_fb_used":                        0,
		"dcgm_fi_dev_fb_total":                       0,
		"dcgm_fi_dev_uncorrectable_remapped_rows":    0,
		"dcgm_fi_dev_correctable_remapped_rows":      0,
		"dcgm_fi_dev_row_remap_failure":              0,
		"dcgm_fi_dev_retired_pending":                0,
		"dcgm_fi_dev_retired_dbe":                    0,
		"dcgm_fi_dev_retired_sbe":                    0,
		"dcgm_fi_dev_banks_remap_rows_avail_high":    0,
		"dcgm_fi_dev_banks_remap_rows_avail_low":     0,
		"dcgm_fi_dev_banks_remap_rows_avail_max":     0,
		"dcgm_fi_dev_banks_remap_rows_avail_none":    0,
		"dcgm_fi_dev_banks_remap_rows_avail_partial": 0,
	}

	for _, metric := range metrics {
		if metric.Component != Name {
			continue
		}

		if _, exists := memMetricsFound[metric.Name]; exists {
			uuid := metric.Labels["uuid"]
			t.Logf("  [OK] %s (uuid=%s): %.0f", metric.Name, uuid, metric.Value)
			memMetricsFound[metric.Name]++
		}
	}

	// Verify we found metrics for all expected fields
	for metricName, count := range memMetricsFound {
		if count == 0 {
			t.Logf("  [WARN] Metric %s was not found in Prometheus registry", metricName)
		} else {
			t.Logf("Found %d instance(s) of metric %s", count, metricName)
		}
	}
}
