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
	"database/sql"
	"fmt"
	"net/url"

	pkgmetadata "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metadata"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/sqlite"
	"github.com/urfave/cli"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/config"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/enrollment"
)

func enrollCommand(cliContext *cli.Context) error {
	gatewayEndpoint := cliContext.String("gateway")
	baseEndpoint := cliContext.String("endpoint")
	sakToken := cliContext.String("token")

	// Validate: exactly one mode must be specified.
	if gatewayEndpoint == "" && (baseEndpoint == "" || sakToken == "") {
		return fmt.Errorf("either --gateway or both --endpoint and --token are required")
	}
	if gatewayEndpoint != "" && (baseEndpoint != "" || sakToken != "") {
		return fmt.Errorf("--gateway and --endpoint/--token are mutually exclusive")
	}

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

	// Idempotency: if a JWT is already stored, enrollment already happened on this node.
	// Skip to avoid redundant backend calls on pod restarts.
	existingJWT, _ := pkgmetadata.ReadMetadata(context.Background(), dbRW, pkgmetadata.MetadataKeyToken)
	if existingJWT != "" {
		fmt.Fprintf(cliContext.App.Writer, "Already enrolled, skipping\n")
		return nil
	}

	if gatewayEndpoint != "" {
		return enrollViaGateway(dbRW, gatewayEndpoint)
	}
	return enrollDirect(dbRW, baseEndpoint, sakToken)
}

// enrollViaGateway handles K8s gateway-proxied enrollment.
// The gateway holds the SAK; the agent calls the gateway proxy endpoint,
// which authenticates with the backend on the agent's behalf and returns a JWT.
// The agent stores only the JWT and the gateway enroll URL (no SAK on the agent).
func enrollViaGateway(dbRW *sql.DB, gatewayEndpoint string) error {
	enrollProxyURL, err := url.JoinPath(gatewayEndpoint, "enroll")
	if err != nil {
		return fmt.Errorf("failed to construct gateway enroll URL: %w", err)
	}

	// No SAK on the agent side — the gateway proxy authenticates with its own SAK.
	jwtToken, err := enrollment.PerformEnrollment(context.Background(), enrollProxyURL, "")
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyToken, jwtToken); err != nil {
		return fmt.Errorf("failed to set JWT token: %w", err)
	}
	if err := pkgmetadata.SetMetadata(ctx, dbRW, "enroll_endpoint", enrollProxyURL); err != nil {
		return fmt.Errorf("failed to set enroll endpoint: %w", err)
	}

	fmt.Println("Enrollment succeeded")
	return nil
}

// enrollDirect handles bare-metal direct enrollment with the backend.
func enrollDirect(dbRW *sql.DB, baseEndpoint, sakToken string) error {
	baseURL, err := url.Parse(baseEndpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}

	if baseURL.Scheme != "https" {
		return fmt.Errorf("enrollment endpoint must use HTTPS, got %s", baseURL.Scheme)
	}

	healthEndpoint, err := url.JoinPath(baseURL.String(), "api", "v1", "health")
	if err != nil {
		return fmt.Errorf("failed to construct health endpoint: %w", err)
	}

	enrollEndpoint, err := url.JoinPath(healthEndpoint, "enroll")
	if err != nil {
		return fmt.Errorf("failed to construct enroll endpoint: %w", err)
	}

	jwtToken, err := enrollment.PerformEnrollment(context.Background(), enrollEndpoint, sakToken)
	if err != nil {
		return err
	}

	metricsEndpoint, err := url.JoinPath(healthEndpoint, "metrics")
	if err != nil {
		return fmt.Errorf("failed to construct metrics endpoint: %w", err)
	}
	logsEndpoint, err := url.JoinPath(healthEndpoint, "logs")
	if err != nil {
		return fmt.Errorf("failed to construct logs endpoint: %w", err)
	}
	nonceEndpoint, err := url.JoinPath(healthEndpoint, "nonce")
	if err != nil {
		return fmt.Errorf("failed to construct nonce endpoint: %w", err)
	}

	return storeConfigInMetadata(dbRW, enrollEndpoint, metricsEndpoint, logsEndpoint, nonceEndpoint, jwtToken, sakToken)
}

func storeConfigInMetadata(dbRW *sql.DB, enrollEndpoint, metricsEndpoint, logsEndpoint, nonceEndpoint, jwtToken, sakToken string) error {
	ctx := context.Background()

	if err := pkgmetadata.SetMetadata(ctx, dbRW, "sak_token", sakToken); err != nil {
		return fmt.Errorf("failed to set SAK token: %w", err)
	}
	if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyToken, jwtToken); err != nil {
		return fmt.Errorf("failed to set JWT token: %w", err)
	}
	if err := pkgmetadata.SetMetadata(ctx, dbRW, "enroll_endpoint", enrollEndpoint); err != nil {
		return fmt.Errorf("failed to set enroll endpoint: %w", err)
	}
	if err := pkgmetadata.SetMetadata(ctx, dbRW, "metrics_endpoint", metricsEndpoint); err != nil {
		return fmt.Errorf("failed to set metrics endpoint: %w", err)
	}
	if err := pkgmetadata.SetMetadata(ctx, dbRW, "logs_endpoint", logsEndpoint); err != nil {
		return fmt.Errorf("failed to set logs endpoint: %w", err)
	}
	if err := pkgmetadata.SetMetadata(ctx, dbRW, "nonce_endpoint", nonceEndpoint); err != nil {
		return fmt.Errorf("failed to set nonce endpoint: %w", err)
	}
	return nil
}
