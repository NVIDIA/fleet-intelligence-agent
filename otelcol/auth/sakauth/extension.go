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

// Package sakauth implements an OpenTelemetry Collector auth.Client extension
// that exchanges a Service Account Key (SAK) for a short-lived JWT using the
// Fleet Intelligence backend enrollment endpoint, and automatically refreshes
// the token on 401 responses.
//
// It also exposes an enrollment proxy HTTP server so that agents in K8s can
// obtain a JWT without holding a SAK themselves: agents POST to
// http://gateway:<EnrollProxyPort>/enroll and receive {"jwt_assertion":"..."}.
package sakauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension/auth"
	"google.golang.org/grpc/credentials"
)

// enrollResponse mirrors the backend enrollment response.
type enrollResponse struct {
	JWTToken string `json:"jwt_assertion"`
}

// sakAuthExtension implements auth.Client using the SAK→JWT enrollment flow.
type sakAuthExtension struct {
	cfg         *Config
	jwt         string
	mu          sync.RWMutex
	proxyServer *http.Server
}

// Ensure the extension satisfies the auth.Client interface at compile time.
var _ auth.Client = (*sakAuthExtension)(nil)

func newSAKAuthExtension(cfg *Config) (*sakAuthExtension, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &sakAuthExtension{cfg: cfg}, nil
}

// Start fetches the initial JWT and, if EnrollProxyPort > 0, starts the
// enrollment proxy so agents can obtain JWTs without holding a SAK.
func (e *sakAuthExtension) Start(ctx context.Context, _ component.Host) error {
	jwt, err := e.performEnrollment(ctx)
	if err != nil {
		return fmt.Errorf("sakauth: initial enrollment failed: %w", err)
	}
	e.mu.Lock()
	e.jwt = jwt
	e.mu.Unlock()

	if e.cfg.EnrollProxyPort > 0 {
		if err := e.startEnrollProxy(); err != nil {
			return fmt.Errorf("sakauth: failed to start enrollment proxy: %w", err)
		}
	}

	return nil
}

// Shutdown stops the enrollment proxy if it was started.
func (e *sakAuthExtension) Shutdown(ctx context.Context) error {
	if e.proxyServer != nil {
		return e.proxyServer.Shutdown(ctx)
	}
	return nil
}

// RoundTripper returns an http.RoundTripper that injects the current JWT into
// each outbound request and refreshes it on a 401 response.
func (e *sakAuthExtension) RoundTripper(base http.RoundTripper) (http.RoundTripper, error) {
	return &sakRoundTripper{base: base, ext: e}, nil
}

// PerRPCCredentials returns nil — this extension only supports HTTP, not gRPC.
func (e *sakAuthExtension) PerRPCCredentials() (credentials.PerRPCCredentials, error) {
	return nil, fmt.Errorf("sakauth: gRPC transport is not supported; use the otlphttp exporter")
}

// startEnrollProxy starts the HTTP server that proxies enrollment for agents.
func (e *sakAuthExtension) startEnrollProxy() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/enroll", e.handleEnroll)

	addr := fmt.Sprintf("0.0.0.0:%d", e.cfg.EnrollProxyPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	e.proxyServer = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		if err := e.proxyServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			_ = err
		}
	}()

	return nil
}

// handleEnroll proxies an agent enrollment request to the backend.
// The agent sends POST /enroll with no credentials. The gateway authenticates
// with its own SAK and returns the resulting JWT to the agent.
func (e *sakAuthExtension) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jwt, err := e.performEnrollment(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("enrollment failed: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(enrollResponse{JWTToken: jwt})
}

func (e *sakAuthExtension) getJWT() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.jwt
}

func (e *sakAuthExtension) refreshJWT(ctx context.Context) (string, error) {
	jwt, err := e.performEnrollment(ctx)
	if err != nil {
		return "", err
	}
	e.mu.Lock()
	e.jwt = jwt
	e.mu.Unlock()
	return jwt, nil
}

// performEnrollment calls the backend enrollment endpoint with the SAK and
// returns the JWT from the response. This replicates the logic in
// internal/enrollment/enrollment.go without importing the internal package.
func (e *sakAuthExtension) performEnrollment(ctx context.Context) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.EnrollEndpoint, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build enrollment request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.cfg.SAKToken)
	req.Header.Set("User-Agent", "fleetint-otelcol")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("enrollment request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read enrollment response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("enrollment returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result enrollResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse enrollment response: %w", err)
	}
	if result.JWTToken == "" {
		return "", fmt.Errorf("enrollment response missing jwt_assertion field")
	}
	return result.JWTToken, nil
}

// sakRoundTripper injects the JWT and handles transparent token refresh on 401.
type sakRoundTripper struct {
	base http.RoundTripper
	ext  *sakAuthExtension
}

func (rt *sakRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+rt.ext.getJWT())

	resp, err := rt.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	// JWT expired — refresh and retry once.
	resp.Body.Close()
	newJWT, err := rt.ext.refreshJWT(req.Context())
	if err != nil {
		return nil, fmt.Errorf("sakauth: token refresh after 401 failed: %w", err)
	}

	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+newJWT)
	return rt.base.RoundTrip(req)
}
