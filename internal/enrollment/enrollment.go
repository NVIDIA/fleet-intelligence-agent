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

// Package enrollment provides shared enrollment functionality for the Fleet Intelligence agent
package enrollment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/leptonai/gpud/pkg/log"
)

// EnrollResponse represents the response from the enrollment endpoint
type EnrollResponse struct {
	JWTToken string `json:"jwt_assertion"`
}

// PerformEnrollment performs the enrollment request to get a new JWT token
func PerformEnrollment(ctx context.Context, enrollEndpoint, sakToken string) (string, error) {
	if enrollEndpoint == "" {
		return "", fmt.Errorf("enrollEndpoint cannot be empty")
	}
	if sakToken == "" {
		return "", fmt.Errorf("sakToken cannot be empty")
	}

	// Use the provided enrollment endpoint directly
	enrollURL := enrollEndpoint

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create HTTP request with empty body
	req, err := http.NewRequestWithContext(ctx, "POST", enrollURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers (no Content-Type since no body is sent)
	req.Header.Set("User-Agent", "fleet-intelligence-agent")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", sakToken))

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		log.Logger.Errorw("Enrollment request failed", "error", err, "url", enrollURL)
		return "", fmt.Errorf("failed to make enrollment request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Logger.Errorw("Failed to read enrollment response body", "error", err)
		return "", fmt.Errorf("failed to read enrollment response: %w", err)
	}

	// Check response status and return specific error messages
	if resp.StatusCode != http.StatusOK {
		var errMsg string
		switch resp.StatusCode {
		case http.StatusBadRequest: // 400
			errMsg = "The token used in the enrollment is not in the correct format. Please check the token. If all else fails, generate a new token by going to the UI"
		case http.StatusUnauthorized: // 401
			errMsg = "The token used in the enrollment is incorrect. Please generate a new token by going to the UI or make sure you are using the correct token"
		case http.StatusForbidden: // 403
			errMsg = "The token used in the enrollment is incorrect/expired. Please generate a new token by going to the UI or make sure you are using the correct token"
		case http.StatusNotFound: // 404
			errMsg = "The endpoint is not found. Please use the correct endpoint"
		case http.StatusTooManyRequests: // 429
			errMsg = "Please retry after some time. Server is under heavy load"
		case http.StatusBadGateway: // 502
			errMsg = "Some temporary issue caused enrollment to fail. Please try again"
		case http.StatusServiceUnavailable: // 503
			errMsg = "Service is unavailable currently. Please try again"
		case http.StatusGatewayTimeout: // 504
			errMsg = "Service is experiencing load and is slow to respond. Please try again maybe after a few minutes"
		default:
			errMsg = fmt.Sprintf("enrollment request failed with status %d", resp.StatusCode)
		}

		// Print error to stderr
		fmt.Fprintf(os.Stderr, "Enrollment failed: %s\n", errMsg)
		return "", fmt.Errorf("%s", errMsg)
	}

	// Parse response
	var enrollResp EnrollResponse
	if err := json.Unmarshal(respBody, &enrollResp); err != nil {
		log.Logger.Errorw("Failed to parse enrollment response JSON", "error", err)
		return "", fmt.Errorf("failed to parse enrollment response: %w", err)
	}

	// Validate JWT token is present
	if enrollResp.JWTToken == "" {
		log.Logger.Errorw("Enrollment response missing jwt-token field")
		return "", fmt.Errorf("enrollment response missing jwt-token field")
	}

	// Print success to stdout
	fmt.Fprintf(os.Stdout, "Enrollment succeeded\n")
	return enrollResp.JWTToken, nil
}
