// SPDX-FileCopyrightText: Copyright (c) 2025, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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

package attestation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgmetadata "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metadata"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/config"
)

func useMissingStateFile(t *testing.T) {
	t.Helper()

	orig := defaultStateFileFn
	defaultStateFileFn = func() (string, error) {
		return filepath.Join(t.TempDir(), "missing", "fleetint.state"), nil
	}
	t.Cleanup(func() {
		defaultStateFileFn = orig
	})
}

func TestManager_NewManager(t *testing.T) {
	ctx := context.Background()

	cfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: false,
	}
	manager := NewManager(ctx, nil, cfg) // nil nvmlInstance, 20s for testing, no jitter
	require.NotNil(t, manager)
	assert.NotNil(t, manager.ctx)
	assert.NotNil(t, manager.cancel)
	assert.NotNil(t, manager.cache)
	assert.Nil(t, manager.nvmlInstance) // Should be nil as passed
}

func TestManager_StartStop(t *testing.T) {
	ctx := context.Background()
	cfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: false,
	}
	manager := NewManager(ctx, nil, cfg)

	// Start should not block (Start() doesn't return error)
	manager.Start()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Stop should work cleanly
	manager.Stop()

	// Verify context is canceled
	select {
	case <-manager.ctx.Done():
		// Expected - context should be canceled
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected context to be canceled after Stop()")
	}
}

func TestManager_GetAttestationData(t *testing.T) {
	ctx := context.Background()
	cfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: false,
	}
	manager := NewManager(ctx, nil, cfg)

	// Initially should have empty data
	attestationData := manager.GetAttestationData()
	assert.Nil(t, attestationData)

	// Manually update cache to test getter
	testAttestationData := &AttestationData{
		SDKResponse: AttestationSDKResponse{
			Evidences: []EvidenceItem{
				{
					Arch:          "turing",
					Certificate:   "test_cert",
					DriverVersion: "550.90.07",
					Evidence:      "test_evidence",
					Nonce:         "test_nonce",
					VBIOSVersion:  "90.17.A9.00.0B",
					Version:       "1.0",
				},
			},
			ResultCode:    0,
			ResultMessage: "Ok",
		},
		NonceRefreshTimestamp: time.Now().UTC(),
	}

	manager.cache.updateAttestationData(testAttestationData)

	// Now should return the test data
	attestationData = manager.GetAttestationData()
	assert.Equal(t, testAttestationData, attestationData)
}

func TestManager_IsAttestationDataUpdated(t *testing.T) {
	ctx := context.Background()
	cfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: false,
	}
	manager := NewManager(ctx, nil, cfg)

	baseTime := time.Now().UTC()

	// Initially, no updates
	assert.False(t, manager.IsAttestationDataUpdated(baseTime))

	// Update the cache
	testData := &AttestationData{
		SDKResponse: AttestationSDKResponse{
			Evidences: []EvidenceItem{{
				Arch:          "turing",
				Certificate:   "test",
				DriverVersion: "550.90.07",
				Evidence:      "test",
				Nonce:         "test_nonce",
				VBIOSVersion:  "90.17.A9.00.0B",
				Version:       "1.0",
			}},
			ResultCode:    0,
			ResultMessage: "Ok",
		},
		NonceRefreshTimestamp: time.Now().UTC(),
	}
	manager.cache.updateAttestationData(testData)

	// Should now show as updated since baseTime
	assert.True(t, manager.IsAttestationDataUpdated(baseTime))

	// But not updated compared to a future time
	futureTime := time.Now().Add(1 * time.Hour)
	assert.False(t, manager.IsAttestationDataUpdated(futureTime))
}

func TestManager_GetMachineId_NoDatabase(t *testing.T) {
	useMissingStateFile(t)

	ctx := context.Background()
	cfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: false,
	}
	manager := NewManager(ctx, nil, cfg)

	// getMachineId will fail because there's no database with machine ID in test environment
	machineId, err := manager.getMachineId()

	// Expected to fail in test environment without proper database setup
	assert.Error(t, err)
	assert.Empty(t, machineId)
}

// Define the response struct for testing
type testNonceResponse struct {
	Nonce                 string    `json:"nonce"`
	NonceRefreshTimestamp time.Time `json:"nonceRefreshTimestamp"`
	Error                 string    `json:"error,omitempty"`
}

func useDefaultTransport(t *testing.T, transport http.RoundTripper) {
	t.Helper()

	orig := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() {
		http.DefaultTransport = orig
	})
}

func TestManager_GetNonce_MockHTTP(t *testing.T) {
	// Test the nonce parsing logic without actually calling the private method
	// This tests the HTTP interaction pattern

	expectedNonce := "test-nonce-12345"
	expectedTimestamp := time.Now().UTC()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and content type
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Verify Bearer authorization header
		authHeader := r.Header.Get("Authorization")
		assert.Equal(t, "Bearer test-jwt-token", authHeader, "Should have Bearer authorization header")

		// Verify request body (should only contain uuid, not token)
		var requestBody map[string]string
		err := json.NewDecoder(r.Body).Decode(&requestBody)
		require.NoError(t, err)
		assert.Equal(t, "test-machine-id", requestBody["uuid"])
		assert.NotContains(t, requestBody, "token", "Token should not be in request body when using Bearer auth")

		// Send successful response
		response := testNonceResponse{
			Nonce:                 expectedNonce,
			NonceRefreshTimestamp: expectedTimestamp,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Test HTTP request/response parsing manually with Bearer auth
	url := server.URL + "/nonce"
	requestBody, err := json.Marshal(map[string]string{
		"uuid": "test-machine-id",
	})
	require.NoError(t, err)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-jwt-token")

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var response testNonceResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, expectedNonce, response.Nonce)
	assert.Equal(t, expectedTimestamp.Unix(), response.NonceRefreshTimestamp.Unix())
}

func TestManager_GetNonce_ServerError(t *testing.T) {
	// Test server error response parsing
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := testNonceResponse{
			Error: "Invalid token",
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Test error response parsing with Bearer auth
	url := server.URL + "/nonce"
	requestBody, err := json.Marshal(map[string]string{
		"uuid": "test-machine-id",
	})
	require.NoError(t, err)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer invalid-token")

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var response testNonceResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "Invalid token", response.Error)
	assert.Empty(t, response.Nonce)
	assert.True(t, response.NonceRefreshTimestamp.IsZero())
}

func TestManager_GetValidatedNonceEndpoint_DerivesFromStoredBackendBaseURL(t *testing.T) {
	manager := newTestManager(t)
	stateFile := setupAttestationMetadataDB(t, map[string]string{
		"backend_base_url": "https://backend.example.com",
	})
	useTestStateFile(t, stateFile)

	got, err := manager.getValidatedNonceEndpoint(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "https://backend.example.com/api/v1/health/nonce", got)
}

func TestManager_GetValidatedNonceEndpoint_RejectsInvalidStoredBackendBaseURL(t *testing.T) {
	manager := newTestManager(t)
	stateFile := setupAttestationMetadataDB(t, map[string]string{
		"backend_base_url": "http://evil.example.com",
	})
	useTestStateFile(t, stateFile)

	_, err := manager.getValidatedNonceEndpoint(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid backend endpoint")
	assert.Contains(t, err.Error(), "https")
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()

	ctx := context.Background()
	cfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: false,
	}
	return NewManager(ctx, nil, cfg)
}

func setupAttestationMetadataDB(t *testing.T, entries map[string]string) string {
	t.Helper()

	stateFile := filepath.Join(t.TempDir(), "fleetint.state")
	db, err := sqlite.Open(stateFile)
	require.NoError(t, err)

	err = pkgmetadata.CreateTableMetadata(context.Background(), db)
	require.NoError(t, err)

	for key, value := range entries {
		err = pkgmetadata.SetMetadata(context.Background(), db, key, value)
		require.NoError(t, err)
	}

	err = db.Close()
	require.NoError(t, err)

	return stateFile
}

func useTestStateFile(t *testing.T, stateFile string) {
	t.Helper()

	orig := defaultStateFileFn
	defaultStateFileFn = func() (string, error) {
		return stateFile, nil
	}
	t.Cleanup(func() {
		defaultStateFileFn = orig
	})
}

func TestManager_GetEvidences(t *testing.T) {
	ctx := context.Background()
	cfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: false,
	}
	manager := NewManager(ctx, nil, cfg)

	testNonce := "931d8dd0add203ac3d8b4fbde75e115278eefcdceac5b87671a748f32364dfcb"

	sdkResponse, err := manager.getEvidences(testNonce)

	// In CI environment, the nvattest binary might not exist or directory might not exist
	if err != nil {
		// If binary is missing, this is expected in CI
		if strings.Contains(err.Error(), "executable file not found") || strings.Contains(err.Error(), "no such file or directory") || strings.Contains(err.Error(), "not found in PATH") || strings.Contains(err.Error(), "The following arguments were not expected") {
			t.Log("Attestation CLI binary or directory not found (expected in CI)")
			return
		}
		// If it's another error, fail the test
		require.NoError(t, err, "Unexpected error running attestation CLI")
	}

	assert.NotNil(t, sdkResponse)

	// In test environment with binary present but no GPU (or mock), CLI may fail
	// Check for expected real CLI response structure
	if sdkResponse.ResultCode == 0 {
		// Success case (when running on real attestation-capable hardware)
		assert.Equal(t, "Ok", sdkResponse.ResultMessage)
		assert.NotEmpty(t, sdkResponse.Evidences, "Should have evidences on success")
		t.Log("Attestation CLI succeeded - running on attestation-capable hardware")
	} else {
		// Expected failure case (test environment without proper attestation hardware)
		// We expect a structured error response from the CLI
		t.Logf("Attestation CLI failed as expected in test environment: %s (Code: %d)",
			sdkResponse.ResultMessage, sdkResponse.ResultCode)
	}
}

func TestCache_ThreadSafety(t *testing.T) {
	cache := &cache{}

	// Test concurrent reads and writes
	done := make(chan bool, 10)

	// Start multiple goroutines writing to cache
	for i := 0; i < 5; i++ {
		go func(id int) {
			defer func() { done <- true }()

			for j := 0; j < 10; j++ {
				testData := &AttestationData{
					SDKResponse: AttestationSDKResponse{
						Evidences: []EvidenceItem{
							{
								Arch:          "BLACKWELL",
								Certificate:   fmt.Sprintf("cert-%d-%d", id, j),
								DriverVersion: "575.28",
								Evidence:      fmt.Sprintf("evidence-%d-%d", id, j),
								Nonce:         fmt.Sprintf("nonce-%d-%d", id, j),
								VBIOSVersion:  "96.00.AF.00.01",
								Version:       "1.0",
							},
						},
						ResultCode:    0,
						ResultMessage: "Ok",
						//NonceRefreshTimestamp: time.Now().UTC().Add(time.Duration(id*j) * time.Millisecond),
					},
					NonceRefreshTimestamp: time.Now().UTC().Add(time.Duration(id*j) * time.Millisecond),
				}
				cache.updateAttestationData(testData)
				time.Sleep(time.Millisecond) // Small delay to increase chance of concurrent access
			}
		}(i)
	}

	// Start multiple goroutines reading from cache
	for i := 0; i < 5; i++ {
		go func(id int) {
			defer func() { done <- true }()

			for j := 0; j < 10; j++ {
				cache.GetAttestationData()
				baseTime := time.Now().UTC().Add(-time.Duration(j) * time.Second)
				cache.isUpdatedSince(baseTime)
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		select {
		case <-done:
			// Good, goroutine completed
		case <-time.After(15 * time.Second):
			t.Fatal("Goroutines did not complete within timeout - possible deadlock")
		}
	}

	// Verify cache is still functional
	attestationData := cache.GetAttestationData()
	assert.NotNil(t, attestationData) // Should have some data from the concurrent writes
}

func TestManager_RunAttestation_WithFallback(t *testing.T) {
	ctx := context.Background()
	cfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: false,
	}
	manager := NewManager(ctx, nil, cfg)

	// Since runAttestation is called by Start(), we need to test it differently
	// We'll test the fallback behavior by checking that it handles errors gracefully

	// The method should not panic when NVML is not available
	assert.NotPanics(t, func() {
		manager.runAttestation()
	})

	// After running with fallbacks, cache should contain failure information
	attestationData := manager.GetAttestationData()
	if attestationData != nil {
		// If we have attestation data, it should indicate failure
		assert.False(t, attestationData.Success, "Should indicate failure")
		assert.NotEmpty(t, attestationData.ErrorMessage, "Should have error message")
		t.Log("Attestation failed as expected with error:", attestationData.ErrorMessage)
	} else {
		t.Log("No attestation data available - this can happen if attestation manager didn't run")
	}
}

func TestManager_IntegrationTest(t *testing.T) {
	// This is a more comprehensive test that tests the update tracking functionality

	ctx := context.Background()
	cfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: false,
	}
	manager := NewManager(ctx, nil, cfg)

	// Test the update tracking over time
	baseTime := time.Now().UTC()

	// Initially no updates
	assert.False(t, manager.IsAttestationDataUpdated(baseTime))

	// Manually update cache (simulating successful attestation)
	testAttestationData := &AttestationData{
		SDKResponse: AttestationSDKResponse{
			Evidences: []EvidenceItem{
				{
					Arch:          "BLACKWELL",
					Certificate:   "integration-cert",
					DriverVersion: "575.28",
					Evidence:      "integration-evidence",
					Nonce:         "integration-nonce",
					VBIOSVersion:  "96.00.AF.00.01",
					Version:       "1.0",
				},
			},
			ResultCode:    0,
			ResultMessage: "Ok",
		},
		NonceRefreshTimestamp: time.Now().UTC(),
	}
	manager.cache.updateAttestationData(testAttestationData)

	// Now should show updates
	assert.True(t, manager.IsAttestationDataUpdated(baseTime))

	// Data should be retrievable
	attestationData := manager.GetAttestationData()
	assert.Equal(t, testAttestationData, attestationData)

	// Test Start/Stop functionality
	manager.Start()
	time.Sleep(50 * time.Millisecond) // Let it start
	manager.Stop()

	// Context should be done
	select {
	case <-manager.ctx.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Context should be canceled after Stop()")
	}
}

func TestEvidenceItem_JSONSerialization(t *testing.T) {
	evidence := EvidenceItem{
		Evidence:    "test-evidence-data",
		Certificate: "test-certificate-data",
	}

	// Test marshaling
	data, err := json.Marshal(evidence)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "test-evidence-data")
	assert.Contains(t, string(data), "test-certificate-data")

	// Test unmarshaling
	var unmarshaled EvidenceItem
	err = json.Unmarshal(data, &unmarshaled)
	assert.NoError(t, err)
	assert.Equal(t, evidence.Evidence, unmarshaled.Evidence)
	assert.Equal(t, evidence.Certificate, unmarshaled.Certificate)
}

func TestManager_CalculateJitter(t *testing.T) {
	ctx := context.Background()
	cfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: true,
	}
	manager := NewManager(ctx, nil, cfg)

	// Test with 0 max jitter
	jitter := manager.calculateJitter(0)
	assert.Equal(t, time.Duration(0), jitter)

	// Test with positive max jitter
	maxJitter := 100 * time.Millisecond
	for i := 0; i < 10; i++ {
		jitter = manager.calculateJitter(maxJitter)
		assert.GreaterOrEqual(t, jitter, time.Duration(0))
		assert.Less(t, jitter, maxJitter)
	}
}

func TestValidateNonce(t *testing.T) {
	tests := []struct {
		name    string
		nonce   string
		wantErr string
	}{
		{name: "valid_hex", nonce: "abcdef0123456789"},
		{name: "valid_base64", nonce: "dGVzdA=="},
		{name: "valid_base64url", nonce: "abc-def_ghi+jkl/mno="},
		{name: "empty", nonce: "", wantErr: "nonce is empty"},
		{name: "too_long", nonce: strings.Repeat("a", 513), wantErr: "exceeds maximum"},
		{name: "max_length_ok", nonce: strings.Repeat("a", 512)},
		{name: "space", nonce: "abc def", wantErr: "invalid character"},
		{name: "newline", nonce: "abc\ndef", wantErr: "invalid character"},
		{name: "semicolon", nonce: "abc;def", wantErr: "invalid character"},
		{name: "shell_metachar", nonce: "$(whoami)", wantErr: "invalid character"},
		{name: "flag_like_valid_chars", nonce: "--output=/etc/passwd"}, // all chars are in the base64url allowlist; safe because nvattest receives it as --nonce value, not a flag
		{name: "pipe", nonce: "abc|def", wantErr: "invalid character"},
		{name: "null_byte", nonce: "abc\x00def", wantErr: "invalid character"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNonce(tc.nonce)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestManager_RunAttestation_ReturnsRetrySoon(t *testing.T) {
	useMissingStateFile(t)

	// This test verifies that runAttestation returns the correct retry hint
	// When agent is not enrolled, it should return true (retry soon)
	// When there's a real failure, it should return false (normal interval)

	ctx := context.Background()
	cfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 24 * time.Hour},
		JitterEnabled: false,
	}
	manager := NewManager(ctx, nil, cfg)

	// Run attestation - in test environment, this will fail at getMachineId
	// (which is a real failure, not "not enrolled"), so it should return false
	shouldRetrySoon := manager.runAttestation()

	// Since we don't have the metadata database with machine ID in test environment,
	// attestation fails with a real error (not "not enrolled"), so shouldRetrySoon should be false
	assert.False(t, shouldRetrySoon, "Should return false for real failures (not 'not enrolled')")

	// Verify the cache has the failure info
	attestationData := manager.GetAttestationData()
	require.NotNil(t, attestationData)
	assert.False(t, attestationData.Success)
	assert.NotEmpty(t, attestationData.ErrorMessage)

	// The error should NOT be about enrollment
	assert.NotContains(t, attestationData.ErrorMessage, "not enrolled",
		"Error should be about machine ID, not enrollment")
}

func TestRetryInterval_Constant(t *testing.T) {
	// Verify the retry interval constant is set appropriately
	assert.Equal(t, 5*time.Minute, retryInterval,
		"Retry interval should be 5 minutes for quick recovery after enrollment")
}

func TestManager_GetNextInterval(t *testing.T) {
	tests := []struct {
		name             string
		configInterval   time.Duration
		shouldRetrySoon  bool
		expectedInterval time.Duration
	}{
		{
			name:             "normal interval when not retrying",
			configInterval:   24 * time.Hour,
			shouldRetrySoon:  false,
			expectedInterval: 24 * time.Hour,
		},
		{
			name:             "retry interval when config is longer",
			configInterval:   24 * time.Hour,
			shouldRetrySoon:  true,
			expectedInterval: 5 * time.Minute, // retryInterval
		},
		{
			name:             "config interval when config is shorter than retry",
			configInterval:   20 * time.Second,
			shouldRetrySoon:  true,
			expectedInterval: 20 * time.Second, // use config, not retryInterval
		},
		{
			name:             "config interval when config equals retry",
			configInterval:   5 * time.Minute,
			shouldRetrySoon:  true,
			expectedInterval: 5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			cfg := &config.AttestationConfig{
				Interval:      metav1.Duration{Duration: tt.configInterval},
				JitterEnabled: false,
			}
			manager := NewManager(ctx, nil, cfg)

			interval := manager.getNextInterval(tt.shouldRetrySoon)
			assert.Equal(t, tt.expectedInterval, interval)
		})
	}
}

func TestManager_RunAttestationWithJitter_Disabled(t *testing.T) {
	ctx := context.Background()
	cfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: false,
	}
	manager := NewManager(ctx, nil, cfg)

	// Should run immediately (we can't easily verify it ran without mocking runAttestation,
	// but we can ensure it doesn't panic and covers the code path)
	done := make(chan bool)
	go func() {
		manager.runAttestationWithJitter()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(15 * time.Second):
		t.Error("runAttestationWithJitter should return immediately when jitter is disabled")
	}
}

func TestManager_RunAttestationWithJitter_Enabled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := &config.AttestationConfig{
		Interval:      metav1.Duration{Duration: 20 * time.Second},
		JitterEnabled: true,
	}
	manager := NewManager(ctx, nil, cfg)

	// We can't easily wait for the random jitter, but we can verify it respects context cancellation
	done := make(chan bool)
	go func() {
		manager.runAttestationWithJitter()
		done <- true
	}()

	// Cancel context to force exit
	cancel()

	select {
	case <-done:
		// Success
	case <-time.After(15 * time.Second):
		t.Error("runAttestationWithJitter should return when context is canceled")
	}
}
