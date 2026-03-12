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

package sakauth

import "fmt"

// Config holds the configuration for the SAK authentication extension.
type Config struct {
	// EnrollEndpoint is the backend enrollment URL used to exchange the SAK for a JWT.
	// Example: https://backend/api/v1/health/enroll
	EnrollEndpoint string `mapstructure:"enroll_endpoint"`

	// SAKToken is the Service Account Key for the gateway service account.
	// Injected from an environment variable or Kubernetes Secret.
	SAKToken string `mapstructure:"sak_token"`

	// EnrollProxyPort is the port on which the gateway listens for agent enrollment
	// proxy requests. Agents POST to http://gateway:<port>/enroll with no credentials;
	// the gateway authenticates with the backend using its SAK and returns a JWT.
	// Defaults to 4319. Set to 0 to disable the enrollment proxy.
	EnrollProxyPort int `mapstructure:"enroll_proxy_port"`
}

func (c *Config) Validate() error {
	if c.EnrollEndpoint == "" {
		return fmt.Errorf("sakauth: enroll_endpoint is required")
	}
	if c.SAKToken == "" {
		return fmt.Errorf("sakauth: sak_token is required")
	}
	return nil
}
