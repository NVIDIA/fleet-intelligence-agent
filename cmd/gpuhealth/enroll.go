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

package main

import (
	"context"
	"fmt"
	"net/url"

	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/urfave/cli"

	"github.com/NVIDIA/gpuhealth/internal/config"
	"github.com/NVIDIA/gpuhealth/internal/enrollment"
)

func enrollCommand(cliContext *cli.Context) error {
	baseEndpoint := cliContext.String("endpoint")
	sakToken := cliContext.String("token")

	// Validate base endpoint
	baseURL, err := url.Parse(baseEndpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}

	// Enforce HTTPS for security
	if baseURL.Scheme != "https" {
		return fmt.Errorf("enrollment endpoint must use HTTPS, got %s", baseURL.Scheme)
	}

	// Construct enroll endpoint first
	enrollEndpoint, err := url.JoinPath(baseURL.String(), "enroll")
	if err != nil {
		return fmt.Errorf("failed to construct enroll endpoint: %w", err)
	}

	// Make enrollment request to get JWT token
	jwtToken, err := enrollment.PerformEnrollment(context.Background(), enrollEndpoint, sakToken)
	if err != nil {
		// Error already printed to stderr by PerformEnrollment
		return err
	}

	// Construct other endpoints using url.JoinPath for proper URL handling
	metricsEndpoint, err := url.JoinPath(baseURL.String(), "metrics")
	if err != nil {
		return fmt.Errorf("failed to construct metrics endpoint: %w", err)
	}
	logsEndpoint, err := url.JoinPath(baseURL.String(), "logs")
	if err != nil {
		return fmt.Errorf("failed to construct logs endpoint: %w", err)
	}
	nonceEndpoint, err := url.JoinPath(baseURL.String(), "nonce")
	if err != nil {
		return fmt.Errorf("failed to construct nonce endpoint: %w", err)
	}

	// Store endpoints and JWT token in metadata table
	if err := storeConfigInMetadata(enrollEndpoint, metricsEndpoint, logsEndpoint, nonceEndpoint, jwtToken, sakToken); err != nil {
		return fmt.Errorf("failed to store configuration: %w", err)
	}

	return nil
}

func storeConfigInMetadata(enrollEndpoint, metricsEndpoint, logsEndpoint, nonceEndpoint, jwtToken, sakToken string) error {
	stateFile, err := config.DefaultStateFile()
	if err != nil {
		return fmt.Errorf("failed to get state file path: %w", err)
	}

	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state database: %w", err)
	}
	defer dbRW.Close()

	if err := pkgmetadata.CreateTableMetadata(context.Background(), dbRW); err != nil {
		return fmt.Errorf("failed to create metadata table: %w", err)
	}

	// Store SAK token (for JWT refresh), JWT token (for API calls), and all endpoints
	if err := pkgmetadata.SetMetadata(context.Background(), dbRW, "sak_token", sakToken); err != nil {
		return fmt.Errorf("failed to set SAK token: %w", err)
	}
	if err := pkgmetadata.SetMetadata(context.Background(), dbRW, pkgmetadata.MetadataKeyToken, jwtToken); err != nil {
		return fmt.Errorf("failed to set JWT token: %w", err)
	}
	if err := pkgmetadata.SetMetadata(context.Background(), dbRW, "enroll_endpoint", enrollEndpoint); err != nil {
		return fmt.Errorf("failed to set enroll endpoint: %w", err)
	}
	if err := pkgmetadata.SetMetadata(context.Background(), dbRW, "metrics_endpoint", metricsEndpoint); err != nil {
		return fmt.Errorf("failed to set metrics endpoint: %w", err)
	}
	if err := pkgmetadata.SetMetadata(context.Background(), dbRW, "logs_endpoint", logsEndpoint); err != nil {
		return fmt.Errorf("failed to set logs endpoint: %w", err)
	}
	if err := pkgmetadata.SetMetadata(context.Background(), dbRW, "nonce_endpoint", nonceEndpoint); err != nil {
		return fmt.Errorf("failed to set nonce endpoint: %w", err)
	}

	return nil
}
