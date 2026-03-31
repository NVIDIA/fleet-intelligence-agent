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
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateLocalServerURL validates a CLI-supplied URL for the local fleetint daemon.
func ValidateLocalServerURL(raw string) (*url.URL, error) {
	parsed, err := parseURL(raw)
	if err != nil {
		return nil, err
	}
	if err := requireScheme(parsed, "http", "https"); err != nil {
		return nil, err
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
