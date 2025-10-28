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
	log.Logger.Debugw("Getting evidences with nonce and VBIOS versions", "nonce", nonce, "vbios_versions", vbiosVersions)
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
	attestationData.ErrorMessage = ""

	log.Logger.Debugw("Updating attestation cache with successful data",
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
	log.Logger.Debugw("Getting evidences with parameters",
		"nonce", nonce,
		"vbios_versions", vbiosVersions,
		"gpu_count", len(vbiosVersions))

	// TODO: When integrating with actual attestation SDK, replace the mock data with the actual data
	var evidences []EvidenceItem
	evidence := EvidenceItem{
		Arch:          "BLACKWELL",
		Certificate:   "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",
		DriverVersion: "575.28",
		Evidence:      "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",
		Nonce:         nonce,
		VBIOSVersion:  "96.00.AF.00.01",
		Version:       "1.0",
	}
	evidences = append(evidences, evidence)

	return &AttestationSDKResponse{
		Evidences:     evidences,
		ResultCode:    0,
		ResultMessage: "Ok",
	}, nil
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
