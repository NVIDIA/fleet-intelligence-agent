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

	pkgfile "github.com/NVIDIA/fleet-intelligence-sdk/pkg/file"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	pkgmetadata "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metadata"
	nvidianvml "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/nvml"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/sqlite"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/config"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/endpoint"
)

var defaultStateFileFn = config.DefaultStateFile

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
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	cache        *cache
	nvmlInstance nvidianvml.Instance
	config       *config.AttestationConfig
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
func NewManager(ctx context.Context, nvmlInstance nvidianvml.Instance, config *config.AttestationConfig) *Manager {
	cctx, cancel := context.WithCancel(ctx)

	// Use 24 hours as default if not specified or invalid
	if config.Interval.Duration <= 0 {
		config.Interval.Duration = 24 * time.Hour
	}

	return &Manager{
		ctx:          cctx,
		cancel:       cancel,
		cache:        &cache{},
		nvmlInstance: nvmlInstance,
		config:       config,
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

// retryInterval is the shorter interval used when agent is not enrolled yet
const retryInterval = 5 * time.Minute

// Start begins the attestation loop with jitter to prevent thundering herd
// Uses dynamic intervals: shorter retry interval when not enrolled, normal interval otherwise
func (m *Manager) Start() {
	log.Logger.Infow("Starting attestation manager with thundering herd prevention")

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		// Add initial jitter to spread out startup requests (0-30 minutes) if enabled
		var initialJitter time.Duration
		if m.config.JitterEnabled {
			initialJitter = m.calculateJitter(30 * time.Minute)
			log.Logger.Infow("Adding initial startup jitter to prevent thundering herd",
				"jitter_duration", initialJitter)
		} else {
			log.Logger.Infow("Startup jitter disabled for testing")
		}

		// Wait for initial jitter before first attestation
		select {
		case <-m.ctx.Done():
			log.Logger.Infow("Context done during initial jitter")
			return
		case <-time.After(initialJitter):
			// Continue to first attestation
		}

		// Run first attestation with additional jitter
		shouldRetrySoon := m.runAttestationWithJitter()

		// Create ticker with configurable interval (default 24 hours)
		// Will be reset after each attestation based on result
		ticker := time.NewTicker(m.getNextInterval(shouldRetrySoon))
		defer ticker.Stop()

		log.Logger.Infow("Attestation ticker started", "interval", m.getNextInterval(shouldRetrySoon))

		for {
			select {
			case <-m.ctx.Done():
				log.Logger.Infow("Context done, stopping attestation manager")
				return
			case <-ticker.C:
				shouldRetrySoon = m.runAttestationWithJitter()
				nextInterval := m.getNextInterval(shouldRetrySoon)
				ticker.Reset(nextInterval)
			}
		}
	}()
}

// getNextInterval returns the appropriate interval based on whether we should retry soon
func (m *Manager) getNextInterval(shouldRetrySoon bool) time.Duration {
	if shouldRetrySoon {
		// Use the shorter of retryInterval and configured interval
		// to avoid slowing down attestation in fast-retry environments (e.g., testing)
		interval := retryInterval
		if m.config.Interval.Duration < retryInterval {
			interval = m.config.Interval.Duration
		}
		log.Logger.Infow("Agent not enrolled, using retry interval",
			"retry_interval", interval)
		return interval
	}
	log.Logger.Infow("Using normal attestation interval",
		"interval", m.config.Interval.Duration)
	return m.config.Interval.Duration
}

// Stop gracefully shuts down the attestation manager and waits for the
// background goroutine to exit. This ensures that any in-progress call
// to defaultStateFileFn (or any other shared state) finishes before Stop
// returns, which prevents data races in tests and orderly cleanup in production.
func (m *Manager) Stop() {
	log.Logger.Infow("Stopping attestation manager")
	m.cancel()
	m.wg.Wait()
}

// runAttestation performs the attestation process and updates the cache
// Returns true if attestation should be retried soon (e.g., agent not enrolled yet)
func (m *Manager) runAttestation() bool {
	log.Logger.Infow("Starting attestation process")

	// Always update cache with result (success or failure) so server knows status
	attestationData := &AttestationData{}

	// Step 1: Get machine ID
	log.Logger.Debugw("Getting machine ID in Attestation")
	machineId, err := m.getMachineId()
	if err != nil {
		log.Logger.Errorw("Failed to get machine ID in Attestation", "error", err)
		// SDKResponse and NonceRefreshTimestamp are nil
		attestationData.Success = false
		attestationData.ErrorMessage = err.Error()
		m.cache.updateAttestationData(attestationData)
		return false
	}

	log.Logger.Debugw("Machine ID retrieved in Attestation",
		"machine_id", machineId)

	// Step 2: Load JWT token from metadata database
	jwtToken := m.getJWTTokenFromMetadata(m.ctx)
	if jwtToken == "" {
		if endpoint := m.getEndpointFromMetadata(m.ctx); endpoint != "" {
			log.Logger.Errorw("JWT token not found in metadata", "endpoint", endpoint)
			// SDKResponse and NonceRefreshTimestamp are nil
			attestationData.Success = false
			attestationData.ErrorMessage = "JWT token not found in agent metadata while agent is enrolled"
			m.cache.updateAttestationData(attestationData)
			return false
		} else {
			log.Logger.Infow("No backend endpoint found in metadata, agent not enrolled, will retry soon")
			// SDKResponse and NonceRefreshTimestamp are nil
			attestationData.Success = false
			attestationData.ErrorMessage = "No backend endpoint found in metadata, agent is not enrolled"
			m.cache.updateAttestationData(attestationData)
			return true // Retry soon - agent may enroll shortly
		}
	}

	// Step 3: Get nonce by calling the envoy endpoint
	nonce, nonceRefreshTimestamp, err := m.getNonce(jwtToken, machineId)
	if err != nil {
		// if agent is not enrolled, it will return in step 2. so here we can directly return the nonce error
		log.Logger.Errorw("Failed to get nonce", "error", err)
		// SDKResponse and NonceRefreshTimestamp are nil
		attestationData.Success = false
		attestationData.ErrorMessage = err.Error()
		m.cache.updateAttestationData(attestationData)
		return false
	}

	// Update nonce refresh timestamp with actual server response
	attestationData.NonceRefreshTimestamp = nonceRefreshTimestamp

	// Step 4: Get evidences from attestation SDK
	log.Logger.Debugw("Getting evidences with nonce")
	sdkResponse, err := m.getEvidences(nonce)
	if err != nil {
		log.Logger.Errorw("Failed to get evidences from attestation SDK", "error", err)
		// SDKResponse
		attestationData.Success = false
		attestationData.ErrorMessage = err.Error()
		m.cache.updateAttestationData(attestationData)
		return false
	}

	// Success case: populate all data
	attestationData.SDKResponse = *sdkResponse
	attestationData.Success = true
	attestationData.ErrorMessage = ""
	log.Logger.Debugw("Attestation data", "attestation_data", attestationData)

	// Update the attestation cache
	m.cache.updateAttestationData(attestationData)
	return false
}

func (m *Manager) getNonce(jwtToken string, machineId string) (string, time.Time, error) {
	endpointURL, err := m.getValidatedNonceEndpoint(m.ctx)
	if err != nil {
		return "", time.Time{}, err
	}

	// Request payload (only include machine ID, JWT token goes in header)
	requestBody, err := json.Marshal(map[string]string{
		"uuid": machineId,
	})
	if err != nil {
		log.Logger.Debugw("failed to marshal request body in nonce endpoint request", "error", err)
		return "", time.Time{}, err
	}

	// Create HTTP request tied to the manager context so that Stop() cancellation
	// unblocks the request and prevents wg.Wait() from hanging indefinitely.
	req, err := http.NewRequestWithContext(m.ctx, "POST", endpointURL, bytes.NewBuffer(requestBody))
	if err != nil {
		log.Logger.Debugw("failed to create HTTP request in nonce endpoint request", "error", err)
		return "", time.Time{}, err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwtToken))

	// Make the HTTP request. Disable redirects so a compromised backend
	// cannot bounce us to an internal service (SSRF).
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Logger.Debugw("failed to make POST request in nonce endpoint request", "error", err)
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

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("nonce endpoint returned HTTP %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Logger.Debugw("failed to decode response in nonce endpoint request", "error", err)
		return "", time.Time{}, err
	}

	if response.Error != "" {
		log.Logger.Debugw("error from server in nonce endpoint request", "error", response.Error)
		return "", time.Time{}, fmt.Errorf("nonce endpoint returned error: %s", response.Error)
	}

	log.Logger.Debugw("Nonce received from server", "nonce_refresh_timestamp", response.NonceRefreshTimestamp)

	return response.Nonce, response.NonceRefreshTimestamp, nil
}

func (m *Manager) getValidatedNonceEndpoint(ctx context.Context) (string, error) {
	nonceEndpoint := m.getNonceEndpointFromMetadata(ctx)
	if nonceEndpoint == "" {
		return "", fmt.Errorf("nonce endpoint not found in metadata")
	}

	validated, err := endpoint.ValidateBackendEndpoint(nonceEndpoint)
	if err != nil {
		return "", fmt.Errorf("invalid nonce endpoint: %w", err)
	}

	return validated.String(), nil
}

// getJWTTokenFromMetadata retrieves the JWT token from the metadata database
func (m *Manager) getJWTTokenFromMetadata(ctx context.Context) string {
	stateFile, err := defaultStateFileFn()
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

func (m *Manager) getEndpointFromMetadata(ctx context.Context) string {
	stateFile, err := defaultStateFileFn()
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

	// Load endpoint from metadata
	if endpoint, err := pkgmetadata.ReadMetadata(ctx, dbRO, pkgmetadata.MetadataKeyEndpoint); err == nil && endpoint != "" {
		return endpoint
	}

	log.Logger.Debugw("backend endpoint not found in metadata")
	return ""
}

// getNonceEndpointFromMetadata retrieves the nonce endpoint from the metadata database
func (m *Manager) getNonceEndpointFromMetadata(ctx context.Context) string {
	stateFile, err := defaultStateFileFn()
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

func (m *Manager) getMachineId() (string, error) {
	stateFile, err := defaultStateFileFn()
	if err != nil {
		return "", fmt.Errorf("failed to get state file path: %w", err)
	}

	dbRO, err := sqlite.Open(stateFile)
	if err != nil {
		return "", fmt.Errorf("failed to open state database: %w", err)
	}
	defer dbRO.Close()

	machineID, err := pkgmetadata.ReadMachineID(m.ctx, dbRO)
	if err != nil {
		return "", fmt.Errorf("failed to read machine ID from metadata: %w", err)
	}

	if machineID == "" {
		return "", fmt.Errorf("machine ID not found in metadata")
	}

	return machineID, nil
}

// validateNonce verifies that a nonce returned by the backend is safe to forward
// as a command-line argument to nvattest. It enforces an allowlist of characters
// (hex, base64url, and common padding/separator symbols) and a maximum length so
// that a compromised backend cannot craft an argument that exploits nvattest's own
// argument parser.
func validateNonce(nonce string) error {
	if nonce == "" {
		return fmt.Errorf("nonce is empty")
	}
	const maxLen = 512
	if len(nonce) > maxLen {
		return fmt.Errorf("nonce length %d exceeds maximum of %d characters", len(nonce), maxLen)
	}
	for i, c := range nonce {
		switch {
		case c >= '0' && c <= '9',
			c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c == '-', c == '_', c == '=', c == '+', c == '/':
			// allowed
		default:
			return fmt.Errorf("nonce contains invalid character %q at position %d", c, i)
		}
	}
	return nil
}

func (m *Manager) getEvidences(nonce string) (*AttestationSDKResponse, error) {
	if err := validateNonce(nonce); err != nil {
		return nil, fmt.Errorf("invalid nonce received from backend: %w", err)
	}

	log.Logger.Infow("Calling attestation SDK CLI")

	// Execute nvattest command
	// Set timeout to prevent hanging, derived from manager context to respect cancellation
	ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
	defer cancel()

	nvattestPath, err := pkgfile.LocateExecutable("nvattest")
	if err != nil {
		return nil, fmt.Errorf("failed to locate attestation CLI: %w", err)
	}
	cmd := exec.CommandContext(ctx, nvattestPath, "collect-evidence", "--gpu-evidence-source=corelib", "--nonce", nonce, "--gpu-architecture", "blackwell", "--format", "json")

	// Capture stdout (JSON) and stderr (logs) separately
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	log.Logger.Debugw("Attestation CLI completed", "exit_error", runErr, "stdout", stdout.String(), "stderr", stderr.String())

	if runErr != nil {
		// If stdout is empty, it means the command failed completely (e.g. command not found)
		if stdout.Len() == 0 {
			return nil, fmt.Errorf("attestation CLI execution failed: %w (stderr: %s)", runErr, stderr.String())
		}
		// If stdout has content, we continue to try parsing the JSON response
		log.Logger.Warnw("Attestation CLI returned error exit code but has output, attempting to parse", "error", runErr)
	}

	// Parse the JSON response from stdout (clean JSON without log messages)
	var response AttestationSDKResponse
	if parseErr := json.Unmarshal(stdout.Bytes(), &response); parseErr != nil {
		log.Logger.Debugw("Failed to parse attestation CLI JSON response", "parse_error", parseErr)
		return nil, fmt.Errorf("failed to parse CLI response: %w (stderr: %s, stdout: %s, exec: %v)", parseErr, stderr.String(), stdout.String(), runErr)
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
// Returns true if attestation should be retried soon (e.g., agent not enrolled yet)
func (m *Manager) runAttestationWithJitter() bool {
	if !m.config.JitterEnabled {
		log.Logger.Infow("Running attestation immediately (jitter disabled)")
		return m.runAttestation()
	}

	// Add significant jitter (0–30 minutes) for each request to spread load across a window,
	// reducing thundering herd risk across many agents.
	requestJitter := m.calculateJitter(30 * time.Minute)
	log.Logger.Infow("Adding request jitter for thundering herd prevention",
		"jitter_duration", requestJitter,
		"max_jitter", "30 minutes")

	select {
	case <-m.ctx.Done():
		return false
	case <-time.After(requestJitter):
		return m.runAttestation()
	}
}
