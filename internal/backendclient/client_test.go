// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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

package backendclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClient_Enroll(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/agent/enroll", r.URL.Path)
		require.Equal(t, "Bearer sak-token", r.Header.Get("Authorization"))
		require.Equal(t, userAgent, r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"jwtAssertion": "jwt-token"})
	}))
	defer server.Close()

	c := NewWithHTTPClient(mustParseURL(t, server.URL), server.Client())

	jwt, err := c.Enroll(context.Background(), "sak-token")
	require.NoError(t, err)
	require.Equal(t, "jwt-token", jwt)
}

func TestNew(t *testing.T) {
	t.Parallel()

	c, err := New("https://backend.example.com")
	require.NoError(t, err)
	require.NotNil(t, c)
}

func TestClient_UpsertNode(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		require.Equal(t, "/v1/agent/nodes/node-1", r.URL.Path)
		require.Equal(t, "Bearer jwt-token", r.Header.Get("Authorization"))

		var req NodeUpsertRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "node-1", req.Hostname)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewWithHTTPClient(mustParseURL(t, server.URL), server.Client())
	err := c.UpsertNode(context.Background(), "node-1", &NodeUpsertRequest{Hostname: "node-1"}, "jwt-token")
	require.NoError(t, err)
}

func TestClient_GetNonce(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/agent/nodes/node-1/nonce", r.URL.Path)
		require.Equal(t, "Bearer jwt-token", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(NonceResponse{
			Nonce:        "abc123",
			JWTAssertion: "new-jwt",
		})
	}))
	defer server.Close()

	c := NewWithHTTPClient(mustParseURL(t, server.URL), server.Client())
	resp, err := c.GetNonce(context.Background(), "node-1", "jwt-token")
	require.NoError(t, err)
	require.Equal(t, "abc123", resp.Nonce)
	require.Equal(t, "new-jwt", resp.JWTAssertion)
}

func TestClient_SubmitAttestation(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/agent/nodes/node-1/attestation", r.URL.Path)
		require.Equal(t, "Bearer jwt-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewWithHTTPClient(mustParseURL(t, server.URL), server.Client())
	err := c.SubmitAttestation(context.Background(), "node-1", &AttestationRequest{}, "jwt-token")
	require.NoError(t, err)
}

func TestClient_RefreshToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/agent/token", r.URL.Path)

		var req struct {
			JWTAssertion string `json:"jwtAssertion"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "jwt-token", req.JWTAssertion)

		_ = json.NewEncoder(w).Encode(map[string]string{"jwtAssertion": "new-jwt-token"})
	}))
	defer server.Close()

	c := NewWithHTTPClient(mustParseURL(t, server.URL), server.Client())
	jwt, err := c.RefreshToken(context.Background(), "jwt-token")
	require.NoError(t, err)
	require.Equal(t, "new-jwt-token", jwt)
}

func TestClient_EnrollMapsHTTPStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad token", http.StatusUnauthorized)
	}))
	defer server.Close()

	c := NewWithHTTPClient(mustParseURL(t, server.URL), server.Client())
	_, err := c.Enroll(context.Background(), "sak-token")
	require.Error(t, err)
	require.Contains(t, err.Error(), "incorrect")
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return parsed
}
