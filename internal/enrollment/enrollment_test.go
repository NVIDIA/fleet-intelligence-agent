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

package enrollment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerformEnrollment_Success(t *testing.T) {
	expectedToken := "test-jwt-token-12345"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		assert.Equal(t, "POST", r.Method)

		// Verify headers
		assert.Equal(t, "fleet-intelligence-agent", r.Header.Get("User-Agent"))
		assert.Equal(t, "Bearer test-sak-token", r.Header.Get("Authorization"))

		// Send successful response
		response := EnrollResponse{
			JWTToken: expectedToken,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	}))
	defer server.Close()

	ctx := context.Background()
	token, err := PerformEnrollment(ctx, server.URL, "test-sak-token")

	require.NoError(t, err)
	assert.Equal(t, expectedToken, token)
}

func TestPerformEnrollment_EmptyEndpoint(t *testing.T) {
	ctx := context.Background()
	token, err := PerformEnrollment(ctx, "", "test-sak-token")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "enrollEndpoint cannot be empty")
	assert.Empty(t, token)
}

func TestPerformEnrollment_EmptyToken(t *testing.T) {
	ctx := context.Background()
	token, err := PerformEnrollment(ctx, "http://example.com", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "sakToken cannot be empty")
	assert.Empty(t, token)
}

func TestPerformEnrollment_HTTPStatusCodes(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		expectedErrMsg string
	}{
		{
			name:           "BadRequest_400",
			statusCode:     http.StatusBadRequest,
			expectedErrMsg: "The token used in the enrollment is not in the correct format",
		},
		{
			name:           "Unauthorized_401",
			statusCode:     http.StatusUnauthorized,
			expectedErrMsg: "The token used in the enrollment is incorrect",
		},
		{
			name:           "Forbidden_403",
			statusCode:     http.StatusForbidden,
			expectedErrMsg: "The token used in the enrollment is incorrect/expired",
		},
		{
			name:           "NotFound_404",
			statusCode:     http.StatusNotFound,
			expectedErrMsg: "The endpoint is not found",
		},
		{
			name:           "TooManyRequests_429",
			statusCode:     http.StatusTooManyRequests,
			expectedErrMsg: "Please retry after some time",
		},
		{
			name:           "BadGateway_502",
			statusCode:     http.StatusBadGateway,
			expectedErrMsg: "Some temporary issue caused enrollment to fail",
		},
		{
			name:           "ServiceUnavailable_503",
			statusCode:     http.StatusServiceUnavailable,
			expectedErrMsg: "Service is unavailable currently",
		},
		{
			name:           "GatewayTimeout_504",
			statusCode:     http.StatusGatewayTimeout,
			expectedErrMsg: "Service is experiencing load and is slow to respond",
		},
		{
			name:           "InternalServerError_500",
			statusCode:     http.StatusInternalServerError,
			expectedErrMsg: "enrollment request failed with status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			ctx := context.Background()
			token, err := PerformEnrollment(ctx, server.URL, "test-sak-token")

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErrMsg)
			assert.Empty(t, token)
		})
	}
}

func TestPerformEnrollment_MissingJWTToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Send response without JWT token
		response := EnrollResponse{
			JWTToken: "",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	}))
	defer server.Close()

	ctx := context.Background()
	token, err := PerformEnrollment(ctx, server.URL, "test-sak-token")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "enrollment response missing jwt-token field")
	assert.Empty(t, token)
}

func TestPerformEnrollment_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("invalid json"))
		require.NoError(t, err)
	}))
	defer server.Close()

	ctx := context.Background()
	token, err := PerformEnrollment(ctx, server.URL, "test-sak-token")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse enrollment response")
	assert.Empty(t, token)
}

func TestPerformEnrollment_ResponseTooLarge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(make([]byte, maxEnrollmentResponseSize+1))
		require.NoError(t, err)
	}))
	defer server.Close()

	ctx := context.Background()
	token, err := PerformEnrollment(ctx, server.URL, "test-sak-token")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "enrollment response too large")
	assert.Empty(t, token)
}

func TestPerformEnrollment_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	token, err := PerformEnrollment(ctx, server.URL, "test-sak-token")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to make enrollment request")
	assert.Empty(t, token)
}

func TestPerformEnrollment_ServerUnavailable(t *testing.T) {
	// Use an invalid URL that will fail to connect
	ctx := context.Background()
	token, err := PerformEnrollment(ctx, "http://localhost:99999", "test-sak-token")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to make enrollment request")
	assert.Empty(t, token)
}

func TestPerformEnrollment_RequestBodyEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify no body is sent (Content-Length should be 0 or not set)
		assert.Equal(t, int64(0), r.ContentLength)

		response := EnrollResponse{
			JWTToken: "test-token",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	}))
	defer server.Close()

	ctx := context.Background()
	token, err := PerformEnrollment(ctx, server.URL, "test-sak-token")

	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestEnrollResponse_JSONSerialization(t *testing.T) {
	tests := []struct {
		name     string
		response EnrollResponse
	}{
		{
			name: "with_token",
			response: EnrollResponse{
				JWTToken: "test-jwt-token",
			},
		},
		{
			name: "empty_token",
			response: EnrollResponse{
				JWTToken: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			data, err := json.Marshal(tt.response)
			assert.NoError(t, err)

			// Test unmarshaling
			var unmarshaled EnrollResponse
			err = json.Unmarshal(data, &unmarshaled)
			assert.NoError(t, err)
			assert.Equal(t, tt.response.JWTToken, unmarshaled.JWTToken)
		})
	}
}

func TestPerformEnrollment_MultipleSuccessiveRequests(t *testing.T) {
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		response := EnrollResponse{
			JWTToken: fmt.Sprintf("token-%d", requestCount),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	}))
	defer server.Close()

	ctx := context.Background()

	// Make multiple requests
	for i := 1; i <= 3; i++ {
		token, err := PerformEnrollment(ctx, server.URL, "test-sak-token")
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("token-%d", i), token)
	}

	assert.Equal(t, 3, requestCount)
}
