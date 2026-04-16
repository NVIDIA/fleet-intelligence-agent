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

package endpoint

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ValidateLocalServerURL validates a CLI-supplied URL for the local fleetint daemon.
// It accepts:
//   - A bare absolute path (unix socket):     /run/fleetint/fleetint.sock
//   - A unix:// URL:                          unix:///run/fleetint/fleetint.sock
//   - HTTP/HTTPS URLs with a loopback host:   http://localhost:15133
func ValidateLocalServerURL(raw string) (*url.URL, error) {
	// Bare absolute paths are treated as unix socket paths.
	if strings.HasPrefix(raw, "/") {
		return &url.URL{Scheme: "unix", Path: raw}, nil
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Handle unix:// scheme URLs, enforcing that no host, user, query, or
	// fragment is present so stray input is not silently accepted as part
	// of the socket path.
	if parsed.Scheme == "unix" {
		if parsed.User != nil || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("unix socket URL must not include host, user info, query parameters, or fragments")
		}
		if parsed.Path == "" || !strings.HasPrefix(parsed.Path, "/") {
			return nil, fmt.Errorf("unix socket URL must have an absolute path, got %q", raw)
		}
		return &url.URL{Scheme: "unix", Path: parsed.Path}, nil
	}

	// Re-validate through parseURL for TCP schemes (checks host, user, query, fragment).
	parsed, err = parseURL(raw)
	if err != nil {
		return nil, err
	}
	if err := requireScheme(parsed, "http", "https"); err != nil {
		return nil, err
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return nil, fmt.Errorf("server URL must not include a path, got %q", parsed.Path)
	}

	host := parsed.Hostname()
	if host == "" {
		return nil, fmt.Errorf("server URL must include a host")
	}
	if !isLoopbackHost(host) {
		return nil, fmt.Errorf("server URL host must be localhost or a loopback IP, got %q", host)
	}

	return parsed, nil
}

// NewAgentHTTPClient returns an http.Client configured to reach the fleetint daemon
// described by serverURL. For unix socket URLs it installs a custom dialer; for
// TCP URLs it returns a plain client.
func NewAgentHTTPClient(serverURL *url.URL) *http.Client {
	noRedirect := func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	if serverURL.Scheme == "unix" {
		socketPath := serverURL.Path
		return &http.Client{
			Timeout:       5 * time.Second,
			CheckRedirect: noRedirect,
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
		}
	}
	return &http.Client{Timeout: 5 * time.Second, CheckRedirect: noRedirect}
}

// AgentBaseURL returns the HTTP base URL to use when constructing request URLs.
// For unix socket connections the actual host is irrelevant to the transport, so
// we normalise to http://localhost and route via the custom dialer.
func AgentBaseURL(serverURL *url.URL) *url.URL {
	if serverURL.Scheme == "unix" {
		return &url.URL{Scheme: "http", Host: "localhost"}
	}
	return serverURL
}

// ValidateBackendEndpoint validates a trusted backend HTTPS endpoint.
func ValidateBackendEndpoint(raw string) (*url.URL, error) {
	parsed, err := parseURL(raw)
	if err != nil {
		return nil, err
	}
	if err := requireScheme(parsed, "https"); err != nil {
		return nil, err
	}
	return parsed, nil
}

// JoinPath appends path elements to a validated base URL.
func JoinPath(base *url.URL, elems ...string) (string, error) {
	return url.JoinPath(base.String(), elems...)
}

func parseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("URL must not include user info")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("URL must include a host")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("URL must not include query parameters or fragments")
	}
	return parsed, nil
}

func requireScheme(parsed *url.URL, allowed ...string) error {
	for _, scheme := range allowed {
		if parsed.Scheme == scheme {
			return nil
		}
	}
	return fmt.Errorf("URL scheme must be one of %q, got %q", allowed, parsed.Scheme)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
