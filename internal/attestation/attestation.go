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

// Package attestation provides functionality for GPU attestation
package attestation

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/sqlite"

	"github.com/NVIDIA/gpuhealth/internal/config"
)

// EvidenceItem represents a single evidence item from the attestation SDK
type EvidenceItem struct {
	Arch          string `json:"arch"`
	Certificate   string `json:"certificate"`
	DriverVersion string `json:"driver_version"`
	Evidence      string `json:"evidence"`
	Nonce         string `json:"nonce"`
	VBIOSVersion  string `json:"vbios_version"`
	Version       string `json:"version"`
}

// AttestationSDKResponse represents the complete response from the attestation SDK
type AttestationSDKResponse struct {
	Evidences     []EvidenceItem `json:"evidences"`
	ResultCode    int            `json:"result_code"`
	ResultMessage string         `json:"result_message"`
}

// AttestationData represents the complete attestation information including SDK response and timestamp
type AttestationData struct {
	SDKResponse           AttestationSDKResponse `json:"sdk_response"`
	NonceRefreshTimestamp time.Time              `json:"nonce_refresh_timestamp"`
	Success               bool                   `json:"success"`
	ErrorMessage          string                 `json:"error_message,omitempty"`
}

// Manager manages the attestation process with configurable intervals
type Manager struct {
	ctx                 context.Context
	cancel              context.CancelFunc
	cache               *cache
	nvmlInstance        nvidianvml.Instance
	attestationInterval time.Duration
}

// cache holds the latest attestation results with thread-safe access
type cache struct {
	mu              sync.RWMutex
	attestationData *AttestationData
	lastUpdated     time.Time
}

// GetAttestationData returns the current attestation data thread-safely
func (c *cache) GetAttestationData() *AttestationData {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.attestationData
}

// updateAttestationData updates the attestation cache thread-safely
func (c *cache) updateAttestationData(attestationData *AttestationData) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.attestationData = attestationData
	c.lastUpdated = time.Now().UTC()
}

// NewManager creates a new attestation manager
func NewManager(ctx context.Context, nvmlInstance nvidianvml.Instance, attestationInterval time.Duration) *Manager {
	cctx, cancel := context.WithCancel(ctx)

	// Use 24 hours as default if not specified or invalid
	if attestationInterval <= 0 {
		attestationInterval = 24 * time.Hour
	}

	return &Manager{
		ctx:                 cctx,
		cancel:              cancel,
		cache:               &cache{},
		nvmlInstance:        nvmlInstance,
		attestationInterval: attestationInterval,
	}
}

// GetAttestationData returns the current attestation data directly
func (m *Manager) GetAttestationData() *AttestationData {
	return m.cache.GetAttestationData()
}

// IsAttestationDataUpdated checks if attestation data has been updated since the given time
func (m *Manager) IsAttestationDataUpdated(since time.Time) bool {
	return m.cache.isUpdatedSince(since)
}

// isUpdatedSince checks if the cache has been updated since the given time
func (c *cache) isUpdatedSince(since time.Time) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastUpdated.After(since)
}

// Start begins the 24-hour attestation ticker with jitter to prevent thundering herd
func (m *Manager) Start() {
	log.Logger.Infow("Starting attestation manager with thundering herd prevention")

	go func() {
		// Add initial jitter to spread out startup requests (0-30 minutes)
		initialJitter := m.calculateJitter(30 * time.Minute)
		log.Logger.Infow("Adding initial startup jitter to prevent thundering herd",
			"jitter_duration", initialJitter)

		// Wait for initial jitter before first attestation
		select {
		case <-m.ctx.Done():
			log.Logger.Infow("Context done during initial jitter")
			return
		case <-time.After(initialJitter):
			// Continue to first attestation
		}

		// Run first attestation with additional jitter
		m.runAttestationWithJitter()

		// Create ticker with configurable interval (default 24 hours)
		ticker := time.NewTicker(m.attestationInterval)
		defer ticker.Stop()

		log.Logger.Infow("Attestation ticker started", "interval", m.attestationInterval)

		for {
			select {
			case <-m.ctx.Done():
				log.Logger.Infow("Context done, stopping attestation manager")
				return
			case <-ticker.C:
				m.runAttestationWithJitter()
			}
		}
	}()
}

// Stop gracefully shuts down the attestation manager
func (m *Manager) Stop() {
	log.Logger.Infow("Stopping attestation manager")
	m.cancel()
}

// runAttestation performs the attestation process and updates the cache
func (m *Manager) runAttestation() {
	log.Logger.Infow("Starting attestation process")

	// Always update cache with result (success or failure) so server knows status
	attestationData := &AttestationData{}

	// Step 1: Get VBIOS versions for all GPUs
	log.Logger.Infow("Getting machine ID and VBIOS versions")
	machineId, vbiosVersions, err := m.getMachineIdWithVBIOS()
	if err != nil {
		log.Logger.Errorw("Failed to get VBIOS versions", "error", err)
		attestationData.Success = false
		attestationData.ErrorMessage = err.Error()
		m.cache.updateAttestationData(attestationData)
		return
	}

	log.Logger.Infow("Version information retrieved",
		"machine_id", machineId,
		"vbios_versions", vbiosVersions,
		"gpu_count", len(vbiosVersions))

	// Step 2: Load JWT token from metadata database
	jwtToken := m.getJWTTokenFromMetadata(m.ctx)
	if jwtToken == "" {
		log.Logger.Errorw("JWT token not found in metadata, cannot proceed with attestation")
		attestationData.Success = false
		attestationData.ErrorMessage = "JWT token not found in metadata"
		m.cache.updateAttestationData(attestationData)
		return
	}

	// Step 3: Get nonce by calling the envoy endpoint
	nonce, nonceRefreshTimestamp, err := m.getNonce(jwtToken, machineId)
	if err != nil {
		log.Logger.Errorw("Failed to get nonce", "error", err)
		attestationData.Success = false
		attestationData.ErrorMessage = err.Error()
		m.cache.updateAttestationData(attestationData)
		return
	}

	// Update nonce refresh timestamp with actual server response
	attestationData.NonceRefreshTimestamp = nonceRefreshTimestamp

	// Step 4: Get evidences from attestation SDK
	log.Logger.Infow("Getting evidences with nonce and VBIOS versions", "nonce", nonce, "vbios_versions", vbiosVersions)
	sdkResponse, err := m.getEvidences(nonce, vbiosVersions)
	if err != nil {
		log.Logger.Errorw("Failed to get evidences", "error", err)
		attestationData.Success = false
		attestationData.ErrorMessage = err.Error()
		m.cache.updateAttestationData(attestationData)
		return
	}

	// Success case: populate all data
	attestationData.SDKResponse = *sdkResponse
	attestationData.Success = true
	attestationData.ErrorMessage = "n/a"

	log.Logger.Infow("Updating attestation cache with successful data",
		"evidences_count", len(sdkResponse.Evidences),
		"result_code", sdkResponse.ResultCode,
		"result_message", sdkResponse.ResultMessage,
		"nonce_refresh_timestamp", nonceRefreshTimestamp)

	// Update the attestation cache
	m.cache.updateAttestationData(attestationData)
}

func (m *Manager) getNonce(jwtToken string, machineId string) (string, time.Time, error) {
	// Load nonce endpoint from metadata database
	url := m.getNonceEndpointFromMetadata(m.ctx)
	if url == "" {
		// Return error if nonce endpoint not found in metadata
		return "", time.Time{}, fmt.Errorf("nonce endpoint not found in metadata")
	}

	// Request payload (only include machine ID, JWT token goes in header)
	requestBody, err := json.Marshal(map[string]string{
		"uuid": machineId,
	})
	if err != nil {
		log.Logger.Errorw("Error marshaling request body:", "error", err)
		return "", time.Time{}, err
	}

	// Create HTTP request with proper Bearer authorization
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		log.Logger.Errorw("Error creating HTTP request:", "error", err)
		return "", time.Time{}, err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwtToken))

	// Make the HTTP request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Logger.Errorw("Error making POST request:", "error", err)
		return "", time.Time{}, err
	}
	defer resp.Body.Close()

	// Parsing the response
	var response struct {
		Nonce                 string    `json:"nonce"`
		NonceRefreshTimestamp time.Time `json:"nonce_refresh_timestamp"`
		Error                 string    `json:"error"`
	}

	log.Logger.Debugw("HTTP Response received:",
		"status_code", resp.StatusCode,
		"status", resp.Status,
		"content_type", resp.Header.Get("Content-Type"))

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Logger.Errorw("Error decoding response:", "error", err)
		return "", time.Time{}, err
	}

	if response.Error != "" {
		log.Logger.Errorw("Error from server:", "error", response.Error)
	} else {
		log.Logger.Debugw("Nonce received from server:", "nonce", response.Nonce, "nonce_refresh_timestamp", response.NonceRefreshTimestamp)
	}

	return response.Nonce, response.NonceRefreshTimestamp, nil
}

// getJWTTokenFromMetadata retrieves the JWT token from the metadata database
func (m *Manager) getJWTTokenFromMetadata(ctx context.Context) string {
	stateFile, err := config.DefaultStateFile()
	if err != nil {
		log.Logger.Debugw("failed to get state file path", "error", err)
		return ""
	}

	dbRO, err := sqlite.Open(stateFile)
	if err != nil {
		log.Logger.Debugw("failed to open state database", "error", err)
		return ""
	}
	defer dbRO.Close()

	// Load JWT token from metadata
	if token, err := pkgmetadata.ReadMetadata(ctx, dbRO, pkgmetadata.MetadataKeyToken); err == nil && token != "" {
		return token
	}

	log.Logger.Debugw("JWT token not found in metadata")
	return ""
}

// getNonceEndpointFromMetadata retrieves the nonce endpoint from the metadata database
func (m *Manager) getNonceEndpointFromMetadata(ctx context.Context) string {
	stateFile, err := config.DefaultStateFile()
	if err != nil {
		log.Logger.Debugw("failed to get state file path", "error", err)
		return ""
	}

	dbRO, err := sqlite.Open(stateFile)
	if err != nil {
		log.Logger.Debugw("failed to open state database", "error", err)
		return ""
	}
	defer dbRO.Close()

	// Load nonce endpoint from metadata
	if endpoint, err := pkgmetadata.ReadMetadata(ctx, dbRO, "nonce_endpoint"); err == nil && endpoint != "" {
		return endpoint
	}

	log.Logger.Debugw("Nonce endpoint not found in metadata")
	return ""
}

func (m *Manager) getMachineIdWithVBIOS() (string, []string, error) {
	if m.nvmlInstance == nil {
		log.Logger.Warnw("NVML instance not available, returning mock data")
		return "", nil, fmt.Errorf("NVML instance not available")
	}

	// Get machine info which includes GPU information
	machineInfo, err := pkgmachineinfo.GetMachineInfo(m.nvmlInstance)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get machine info: %w", err)
	}

	if machineInfo.GPUInfo == nil || len(machineInfo.GPUInfo.GPUs) == 0 {
		return "", nil, fmt.Errorf("no GPU information available")
	}

	var vbiosVersions []string
	for i, gpu := range machineInfo.GPUInfo.GPUs {
		// Extract the real VBIOS version from GPU information
		if gpu.VBIOSVersion == "" {
			log.Logger.Warnw("VBIOS version not available for GPU",
				"gpu_index", i,
				"uuid", gpu.UUID,
				"bus_id", gpu.BusID)
			continue
		}

		vbiosVersions = append(vbiosVersions, gpu.VBIOSVersion)

		log.Logger.Debugw("Extracted real VBIOS version for GPU",
			"gpu_index", i,
			"uuid", gpu.UUID,
			"bus_id", gpu.BusID,
			"serial_number", gpu.SN,
			"board_id", gpu.BoardID,
			"vbios_version", gpu.VBIOSVersion)
	}

	return machineInfo.SystemUUID, vbiosVersions, nil
}

func (m *Manager) getEvidences(nonce string, vbiosVersions []string) (*AttestationSDKResponse, error) {
	log.Logger.Infow("Calling attestation SDK CLI", "nonce", nonce)

	// Execute nvattest command
	// Set timeout to prevent hanging, derived from manager context to respect cancellation
	ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "nvattest", "collect-evidence", "--device", "gpu", "--nonce", nonce)

	// Capture stdout (JSON) and stderr (logs) separately
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	log.Logger.Debugw("Attestation CLI completed", "exit_error", err, "stdout", stdout.String(), "stderr", stderr.String())

	if err != nil {
		// If stdout is empty, it means the command failed completely (e.g. not found)
		if stdout.Len() == 0 {
			return nil, fmt.Errorf("attestation CLI execution failed: %w (stderr: %s)", err, stderr.String())
		}
		// If stdout has content, we continue to try parsing the JSON response
		log.Logger.Warnw("Attestation CLI returned error exit code but has output, attempting to parse", "error", err)
	}

	// Parse the JSON response from stdout (clean JSON without log messages)
	var response AttestationSDKResponse
	if parseErr := json.Unmarshal(stdout.Bytes(), &response); parseErr != nil {
		log.Logger.Errorw("Failed to parse attestation CLI JSON response", "parse_error", parseErr)
		return nil, fmt.Errorf("failed to parse CLI response: %w", parseErr)
	}

	log.Logger.Infow("Successfully called attestation SDK",
		"result_code", response.ResultCode,
		"result_message", response.ResultMessage,
		"evidences_count", len(response.Evidences))

	return &response, nil
}

// calculateJitter returns a random duration between 0 and maxJitter to prevent thundering herd
func (m *Manager) calculateJitter(maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}

	// Generate cryptographically secure random number
	maxMs := int64(maxJitter / time.Millisecond)
	if maxMs <= 0 {
		return 0
	}

	randomMs, err := rand.Int(rand.Reader, big.NewInt(maxMs))
	if err != nil {
		log.Logger.Warnw("Failed to generate secure random jitter, using fallback", "error", err)
		// Fallback to simple time-based pseudo-random
		return time.Duration(time.Now().UnixNano()%maxMs) * time.Millisecond
	}

	return time.Duration(randomMs.Int64()) * time.Millisecond
}

// runAttestationWithJitter runs attestation with additional per-request jitter
func (m *Manager) runAttestationWithJitter() {
	// Add significant jitter (0–30 minutes) for each request to spread load across a window,
	// reducing thundering herd risk across many agents.
	requestJitter := m.calculateJitter(30 * time.Minute)
	log.Logger.Infow("Adding request jitter for thundering herd prevention",
		"jitter_duration", requestJitter,
		"max_jitter", "30 minutes")

	select {
	case <-m.ctx.Done():
		return
	case <-time.After(requestJitter):
		m.runAttestation()
	}
}
