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

package dcgm

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
)

func TestResolveInitFromEnv(t *testing.T) {
	// Default: no address override -> TCP localhost.
	t.Setenv("DCGM_URL", "")
	t.Setenv("DCGM_URL_IS_UNIX_SOCKET", "")
	p := resolveInitFromEnv()
	if p.isUnixSocket != "0" || p.address != "localhost" {
		t.Fatalf("expected default tcp localhost, got isUnixSocket=%q address=%q", p.isUnixSocket, p.address)
	}

	// TCP when DCGM_URL is set to host:port
	t.Setenv("DCGM_URL", "dcgm.svc:5555")
	t.Setenv("DCGM_URL_IS_UNIX_SOCKET", "0")
	p = resolveInitFromEnv()
	if p.isUnixSocket != "0" || p.address != "dcgm.svc:5555" {
		t.Fatalf("expected tcp dcgm.svc:5555, got isUnixSocket=%q address=%q", p.isUnixSocket, p.address)
	}

	// DCGM_URL unix socket path with truthy flag.
	t.Setenv("DCGM_URL", "/run/dcgm/dcgm.sock")
	t.Setenv("DCGM_URL_IS_UNIX_SOCKET", "true")
	p = resolveInitFromEnv()
	if p.isUnixSocket != "1" || p.address != "/run/dcgm/dcgm.sock" {
		t.Fatalf("expected unix /run/dcgm/dcgm.sock, got isUnixSocket=%q address=%q", p.isUnixSocket, p.address)
	}

	// Invalid bool values default to tcp.
	t.Setenv("DCGM_URL", "dcgm.svc:5555")
	t.Setenv("DCGM_URL_IS_UNIX_SOCKET", "maybe")
	p = resolveInitFromEnv()
	if p.isUnixSocket != "0" {
		t.Fatalf("expected invalid bool to default to tcp, got isUnixSocket=%q address=%q", p.isUnixSocket, p.address)
	}
}

func TestInstance(t *testing.T) {
	inst, err := New()
	if err != nil {
		t.Fatalf("failed to create instance: %v", err)
	}
	defer inst.Shutdown()

	if !inst.DCGMExists() {
		t.Logf("DCGM not available, skipping detailed tests")
		return
	}

	// Test health check with a system
	err = inst.AddHealthWatch(dcgm.DCGM_HEALTH_WATCH_PCIE)
	if err != nil {
		t.Logf("failed to add health watch: %v", err)
	} else {
		health, incidents, err := inst.HealthCheck(dcgm.DCGM_HEALTH_WATCH_PCIE)
		if err != nil {
			t.Logf("health check failed: %v", err)
		} else {
			t.Logf("health: %v, incidents: %d", health, len(incidents))
		}
	}
}

func TestNoOpInstance(t *testing.T) {
	inst := NewNoOp()

	// Verify no-op behavior
	if inst.DCGMExists() {
		t.Errorf("no-op instance should return false for DCGMExists()")
	}

	// Test HealthCheck returns PASS with no error (graceful degradation)
	health, incidents, err := inst.HealthCheck(dcgm.DCGM_HEALTH_WATCH_PCIE)
	if err != nil {
		t.Errorf("no-op instance should not return error for HealthCheck(): %v", err)
	}
	if health != dcgm.DCGM_HEALTH_RESULT_PASS {
		t.Errorf("no-op instance should return PASS (assume healthy), got %v", health)
	}
	if incidents != nil {
		t.Errorf("no-op instance should return nil incidents, got %v", incidents)
	}

	if err := inst.Shutdown(); err != nil {
		t.Errorf("no-op instance should not return error for Shutdown(): %v", err)
	}
}

func TestInstanceWhenDCGMNotAvailable(t *testing.T) {
	// When DCGM is not available, New() should return a no-op instance
	// without error
	inst, err := New()
	if err != nil {
		t.Fatalf("New() should not return error even when DCGM is not available: %v", err)
	}

	// The instance should be valid (either real or no-op)
	if inst == nil {
		t.Fatal("instance should not be nil")
	}

	// Should be safe to call methods on the instance
	_ = inst.DCGMExists()
	_ = inst.Shutdown()
}

func TestNewWithContextReturnsNoOpOnTimeout(t *testing.T) {
	originalNewInstanceFunc := newInstanceFunc
	defer func() {
		newInstanceFunc = originalNewInstanceFunc
	}()

	blocker := make(chan struct{})
	newInstanceFunc = func() (Instance, error) {
		<-blocker
		return &instance{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	inst, err := NewWithContext(ctx)
	if err != nil {
		t.Fatalf("NewWithContext() returned error: %v", err)
	}
	if inst == nil {
		t.Fatal("instance should not be nil")
	}
	if inst.DCGMExists() {
		t.Fatalf("expected no-op instance after timeout")
	}

	close(blocker)
}

func TestNewWithContextReturnsUnderlyingError(t *testing.T) {
	originalNewInstanceFunc := newInstanceFunc
	defer func() {
		newInstanceFunc = originalNewInstanceFunc
	}()

	expectedErr := errors.New("boom")
	newInstanceFunc = func() (Instance, error) {
		return nil, expectedErr
	}

	inst, err := NewWithContext(context.Background())
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
	if inst != nil {
		t.Fatalf("expected nil instance on error")
	}
}

func TestAddHealthWatch(t *testing.T) {
	inst, err := New()
	if err != nil {
		t.Fatalf("failed to create instance: %v", err)
	}
	defer inst.Shutdown()

	if !inst.DCGMExists() {
		t.Skip("DCGM not available, skipping test")
	}

	// Test adding a single health watch
	err = inst.AddHealthWatch(dcgm.DCGM_HEALTH_WATCH_PCIE)
	if err != nil {
		t.Errorf("AddHealthWatch(PCIE) failed: %v", err)
	}

	// Test adding another health watch (should OR together)
	err = inst.AddHealthWatch(dcgm.DCGM_HEALTH_WATCH_THERMAL)
	if err != nil {
		t.Errorf("AddHealthWatch(THERMAL) failed: %v", err)
	}

	// Verify the systems are tracked
	realInst := inst.(*instance)
	realInst.watchedSystemsMu.Lock()
	watchedSystems := realInst.watchedSystems
	realInst.watchedSystemsMu.Unlock()

	expectedSystems := dcgm.DCGM_HEALTH_WATCH_PCIE | dcgm.DCGM_HEALTH_WATCH_THERMAL
	if watchedSystems != expectedSystems {
		t.Errorf("expected watched systems to be 0x%x, got 0x%x", expectedSystems, watchedSystems)
	}
}

func TestRemoveHealthWatch(t *testing.T) {
	inst, err := New()
	if err != nil {
		t.Fatalf("failed to create instance: %v", err)
	}
	defer inst.Shutdown()

	if !inst.DCGMExists() {
		t.Skip("DCGM not available, skipping test")
	}

	// Add multiple health watches
	err = inst.AddHealthWatch(dcgm.DCGM_HEALTH_WATCH_PCIE | dcgm.DCGM_HEALTH_WATCH_THERMAL | dcgm.DCGM_HEALTH_WATCH_POWER)
	if err != nil {
		t.Fatalf("AddHealthWatch failed: %v", err)
	}

	// Remove one health watch
	err = inst.RemoveHealthWatch(dcgm.DCGM_HEALTH_WATCH_THERMAL)
	if err != nil {
		t.Errorf("RemoveHealthWatch(THERMAL) failed: %v", err)
	}

	// Verify the system was removed
	realInst := inst.(*instance)
	realInst.watchedSystemsMu.Lock()
	watchedSystems := realInst.watchedSystems
	realInst.watchedSystemsMu.Unlock()

	expectedSystems := dcgm.DCGM_HEALTH_WATCH_PCIE | dcgm.DCGM_HEALTH_WATCH_POWER
	if watchedSystems != expectedSystems {
		t.Errorf("expected watched systems to be 0x%x after removal, got 0x%x", expectedSystems, watchedSystems)
	}

	// Remove all remaining watches
	err = inst.RemoveHealthWatch(dcgm.DCGM_HEALTH_WATCH_PCIE | dcgm.DCGM_HEALTH_WATCH_POWER)
	if err != nil {
		t.Errorf("RemoveHealthWatch failed: %v", err)
	}

	// Verify all systems removed
	realInst.watchedSystemsMu.Lock()
	watchedSystems = realInst.watchedSystems
	realInst.watchedSystemsMu.Unlock()

	if watchedSystems != 0 {
		t.Errorf("expected all systems to be removed (0), got 0x%x", watchedSystems)
	}
}

func TestHealthCheck(t *testing.T) {
	inst, err := New()
	if err != nil {
		t.Fatalf("failed to create instance: %v", err)
	}
	defer inst.Shutdown()

	if !inst.DCGMExists() {
		t.Skip("DCGM not available, skipping test")
	}

	// Add a health watch before checking
	err = inst.AddHealthWatch(dcgm.DCGM_HEALTH_WATCH_PCIE)
	if err != nil {
		t.Fatalf("AddHealthWatch failed: %v", err)
	}

	// Perform health check for PCIE system
	health, incidents, err := inst.HealthCheck(dcgm.DCGM_HEALTH_WATCH_PCIE)
	if err != nil {
		t.Errorf("HealthCheck() failed: %v", err)
	}

	// Verify response is valid
	t.Logf("Health result: %v", health)
	t.Logf("Number of incidents: %d", len(incidents))
}

func TestHealthCheckCaching(t *testing.T) {
	// Note: Caching is now handled by DCGMHealthCache, not by the instance.
	// This test now verifies that direct HealthCheck calls work correctly.
	inst, err := New()
	if err != nil {
		t.Fatalf("failed to create instance: %v", err)
	}
	defer inst.Shutdown()

	if !inst.DCGMExists() {
		t.Skip("DCGM not available, skipping test")
	}

	// Add a health watch
	err = inst.AddHealthWatch(dcgm.DCGM_HEALTH_WATCH_PCIE)
	if err != nil {
		t.Fatalf("AddHealthWatch failed: %v", err)
	}

	// First call - should perform actual check and parse
	// Make multiple HealthCheck calls and verify they work correctly
	// Note: Each call now performs a fresh DCGM API call since caching is in DCGMHealthCache
	health1, incidents1, err := inst.HealthCheck(dcgm.DCGM_HEALTH_WATCH_PCIE)
	if err != nil {
		t.Fatalf("first HealthCheck() failed: %v", err)
	}
	t.Logf("First call - Health: %v, incidents: %d", health1, len(incidents1))

	// Second call
	health2, incidents2, err := inst.HealthCheck(dcgm.DCGM_HEALTH_WATCH_PCIE)
	if err != nil {
		t.Fatalf("second HealthCheck() failed: %v", err)
	}
	t.Logf("Second call - Health: %v, incidents: %d", health2, len(incidents2))

	// Third call
	health3, incidents3, err := inst.HealthCheck(dcgm.DCGM_HEALTH_WATCH_PCIE)
	if err != nil {
		t.Fatalf("third HealthCheck() failed: %v", err)
	}
	t.Logf("Third call - Health: %v, incidents: %d", health3, len(incidents3))
}

func TestHealthCheckConcurrency(t *testing.T) {
	// Test concurrent HealthCheck calls
	inst, err := New()
	if err != nil {
		t.Fatalf("failed to create instance: %v", err)
	}
	defer inst.Shutdown()

	if !inst.DCGMExists() {
		t.Skip("DCGM not available, skipping test")
	}

	// Add a health watch
	err = inst.AddHealthWatch(dcgm.DCGM_HEALTH_WATCH_PCIE)
	if err != nil {
		t.Fatalf("AddHealthWatch failed: %v", err)
	}

	// Launch multiple goroutines calling HealthCheck simultaneously
	const numGoroutines = 10
	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			_, _, err := inst.HealthCheck(dcgm.DCGM_HEALTH_WATCH_PCIE)
			if err != nil {
				errChan <- err
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		t.Errorf("concurrent HealthCheck() failed: %v", err)
	}

	t.Logf("Successfully performed %d concurrent health checks", numGoroutines)
}

func TestNoOpInstanceNewMethods(t *testing.T) {
	inst := NewNoOp()

	// Test AddHealthWatch is a no-op (returns nil, does nothing)
	err := inst.AddHealthWatch(dcgm.DCGM_HEALTH_WATCH_PCIE)
	if err != nil {
		t.Errorf("no-op instance AddHealthWatch should return nil (graceful no-op): %v", err)
	}

	// Test RemoveHealthWatch is a no-op (returns nil, does nothing)
	err = inst.RemoveHealthWatch(dcgm.DCGM_HEALTH_WATCH_PCIE)
	if err != nil {
		t.Errorf("no-op instance RemoveHealthWatch should return nil (graceful no-op): %v", err)
	}

	// Test HealthCheck returns PASS with no error (DCGM unavailable = can't check = assume healthy)
	health, incidents, err := inst.HealthCheck(dcgm.DCGM_HEALTH_WATCH_PCIE)
	if err != nil {
		t.Errorf("no-op instance HealthCheck should not return error: %v", err)
	}
	if health != dcgm.DCGM_HEALTH_RESULT_PASS {
		t.Errorf("no-op instance should return PASS (assume healthy when can't check), got %v", health)
	}
	if incidents != nil {
		t.Errorf("no-op instance should return nil incidents, got %v", incidents)
	}
}

func TestHealthCheckMultipleSystems(t *testing.T) {
	inst, err := New()
	if err != nil {
		t.Fatalf("failed to create instance: %v", err)
	}
	defer inst.Shutdown()

	if !inst.DCGMExists() {
		t.Skip("DCGM not available, skipping test")
	}

	// Add multiple health watches
	err = inst.AddHealthWatch(dcgm.DCGM_HEALTH_WATCH_PCIE | dcgm.DCGM_HEALTH_WATCH_THERMAL | dcgm.DCGM_HEALTH_WATCH_POWER)
	if err != nil {
		t.Fatalf("AddHealthWatch failed: %v", err)
	}

	// Check each system - they should all share the same cached DCGM call
	// but get parsed results for their specific system
	systems := []struct {
		name   string
		system dcgm.HealthSystem
	}{
		{"PCIE", dcgm.DCGM_HEALTH_WATCH_PCIE},
		{"THERMAL", dcgm.DCGM_HEALTH_WATCH_THERMAL},
		{"POWER", dcgm.DCGM_HEALTH_WATCH_POWER},
	}

	for _, sys := range systems {
		health, incidents, err := inst.HealthCheck(sys.system)
		if err != nil {
			t.Errorf("HealthCheck(%s) failed: %v", sys.name, err)
		}
		t.Logf("%s: health=%v, incidents=%d", sys.name, health, len(incidents))
	}
}
