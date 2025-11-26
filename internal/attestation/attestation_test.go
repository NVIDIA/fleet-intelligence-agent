// SPDX-FileCopyrightText: Copyright (c) 2025, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package attestation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_NewManager(t *testing.T) {
	ctx := context.Background()

	manager := NewManager(ctx, nil, 20*time.Second) // nil nvmlInstance, 20s for testing
	require.NotNil(t, manager)
	assert.NotNil(t, manager.ctx)
	assert.NotNil(t, manager.cancel)
	assert.NotNil(t, manager.cache)
	assert.Nil(t, manager.nvmlInstance) // Should be nil as passed
}

func TestManager_StartStop(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, nil, 20*time.Second)

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
	manager := NewManager(ctx, nil, 20*time.Second)

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
	manager := NewManager(ctx, nil, 20*time.Second)

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

func TestManager_GetMachineIdWithVBIOS_NilNVML(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, nil, 20*time.Second) // nil nvmlInstance

	machineId, vbiosVersions, err := manager.getMachineIdWithVBIOS()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NVML instance not available")
	assert.Empty(t, machineId)
	assert.Nil(t, vbiosVersions)
}

// Define the response struct for testing
type testNonceResponse struct {
	Nonce                 string    `json:"nonce"`
	NonceRefreshTimestamp time.Time `json:"nonceRefreshTimestamp"`
	Error                 string    `json:"error,omitempty"`
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

func TestManager_GetEvidences(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, nil, 20*time.Second)

	testNonce := "931d8dd0add203ac3d8b4fbde75e115278eefcdceac5b87671a748f32364dfcb"
	testVbiosVersions := []string{"96.00.AF.00.01"}

	sdkResponse, err := manager.getEvidences(testNonce, testVbiosVersions)

	// In CI environment, the nvattest binary might not exist
	if err != nil {
		// If binary is missing, this is expected in CI
		if strings.Contains(err.Error(), "executable file not found") {
			t.Log("Attestation CLI binary not found (expected in CI)")
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
		assert.NotEqual(t, 0, sdkResponse.ResultCode, "Expected non-zero result code on failure")
		assert.NotEmpty(t, sdkResponse.ResultMessage, "Expected error message")
		assert.Empty(t, sdkResponse.Evidences, "Should have no evidences on failure")
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
		case <-time.After(5 * time.Second):
			t.Fatal("Goroutines did not complete within timeout - possible deadlock")
		}
	}

	// Verify cache is still functional
	attestationData := cache.GetAttestationData()
	assert.NotNil(t, attestationData) // Should have some data from the concurrent writes
}

func TestManager_RunAttestation_WithFallback(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, nil, 20*time.Second) // nil nvmlInstance will trigger fallback

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
	manager := NewManager(ctx, nil, 20*time.Second)

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
