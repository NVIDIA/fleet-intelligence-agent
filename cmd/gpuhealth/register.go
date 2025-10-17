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
)

func registerCommand(cliContext *cli.Context) error {
	baseEndpoint := cliContext.String("endpoint")
	token := cliContext.String("token")

	// Validate base endpoint
	baseURL, err := url.Parse(baseEndpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}

	// Construct full endpoints with /api/v1/health prefix
	metricsEndpoint := baseURL.JoinPath("/api/v1/health/metrics").String()
	logsEndpoint := baseURL.JoinPath("/api/v1/health/logs").String()

	// Store endpoints and token in metadata table
	if err := storeConfigInMetadata(metricsEndpoint, logsEndpoint, token); err != nil {
		return fmt.Errorf("failed to store configuration: %w", err)
	}

	return nil
}

func storeConfigInMetadata(metricsEndpoint, logsEndpoint, token string) error {
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
	if err := pkgmetadata.SetMetadata(context.Background(), dbRW, pkgmetadata.MetadataKeyToken, token); err != nil {
		return fmt.Errorf("failed to set auth token: %w", err)
	}
	if err := pkgmetadata.SetMetadata(context.Background(), dbRW, "metrics_endpoint", metricsEndpoint); err != nil {
		return fmt.Errorf("failed to set metrics endpoint: %w", err)
	}
	if err := pkgmetadata.SetMetadata(context.Background(), dbRW, "logs_endpoint", logsEndpoint); err != nil {
		return fmt.Errorf("failed to set logs endpoint: %w", err)
	}

	return nil
}
