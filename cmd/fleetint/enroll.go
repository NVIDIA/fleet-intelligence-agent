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
	"io"
	"os"
	"strings"

	pkgmetadata "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metadata"
	nvidianvml "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/nvml"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/sqlite"
	"github.com/urfave/cli"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/agentstate"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/config"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/endpoint"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/enrollment"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/inventory"
	inventorysink "github.com/NVIDIA/fleet-intelligence-agent/internal/inventory/sink"
	inventorysource "github.com/NVIDIA/fleet-intelligence-agent/internal/inventory/source"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/machineinfo"
)

var (
	performEnrollment = func(enrollEndpoint, sakToken string) (string, error) {
		return enrollment.PerformEnrollment(context.Background(), enrollEndpoint, sakToken)
	}
	storeEnrollmentConfig = storeConfigInMetadata
	performInventorySync  = syncInventoryOnce
)

// resolveToken returns the SAK token from --token, --token-file, or stdin.
func resolveToken(cliContext *cli.Context) (string, error) {
	token := strings.TrimSpace(cliContext.String("token"))
	tokenFile := cliContext.String("token-file")

	if token != "" && tokenFile != "" {
		return "", fmt.Errorf("--token and --token-file are mutually exclusive")
	}

	if tokenFile != "" {
		const maxTokenSize = 1 << 20 // 1 MiB -- SAK tokens are small; anything larger is a mistake
		var raw []byte
		var err error
		if tokenFile == "-" {
			raw, err = io.ReadAll(io.LimitReader(os.Stdin, maxTokenSize))
		} else {
			var file *os.File
			file, err = os.Open(tokenFile)
			if err == nil {
				defer file.Close()
				var info os.FileInfo
				info, err = file.Stat()
				if err == nil && info.Size() >= maxTokenSize {
					return "", fmt.Errorf("token file %q exceeds maximum size of %d bytes", tokenFile, maxTokenSize)
				}
				if err == nil {
					raw, err = io.ReadAll(io.LimitReader(file, maxTokenSize))
				}
			}
		}
		if err != nil {
			return "", fmt.Errorf("failed to read token from %q: %w", tokenFile, err)
		}
		if len(raw) >= maxTokenSize {
			return "", fmt.Errorf("token file %q exceeds maximum size of %d bytes", tokenFile, maxTokenSize)
		}
		token = strings.TrimSpace(string(raw))
	}

	if token == "" {
		return "", fmt.Errorf("a token is required: use --token <value> or --token-file <path>")
	}
	return token, nil
}

func enrollCommand(cliContext *cli.Context) error {
	baseEndpoint := cliContext.String("endpoint")
	force := cliContext.Bool("force")

	sakToken, err := resolveToken(cliContext)
	if err != nil {
		return err
	}

	result, err := runPrecheck()
	if err != nil {
		return fmt.Errorf("failed to run precheck: %w", err)
	}
	printPrecheckResult(writerFromContext(cliContext), result)
	if !result.Passed() {
		if !force {
			fmt.Fprintln(writerFromContext(cliContext), "Enrollment skipped: precheck failed")
			return fmt.Errorf("precheck failed")
		}
		fmt.Fprintln(writerFromContext(cliContext), "Proceeding with enrollment because --force was provided")
	}

	baseURL, err := endpoint.ValidateBackendEndpoint(baseEndpoint)
	if err != nil {
		return fmt.Errorf("invalid enrollment endpoint: %w", err)
	}

	// Construct enroll endpoint
	enrollEndpoint, err := endpoint.JoinPath(baseURL, "api", "v1", "health", "enroll")
	if err != nil {
		return fmt.Errorf("failed to construct enroll endpoint: %w", err)
	}

	// Make enrollment request to get JWT token
	jwtToken, err := performEnrollment(enrollEndpoint, sakToken)
	if err != nil {
		// Error already printed to stderr by PerformEnrollment
		return err
	}

	// Construct other endpoints using url.JoinPath for proper URL handling
	metricsEndpoint, err := endpoint.JoinPath(baseURL, "api", "v1", "health", "metrics")
	if err != nil {
		return fmt.Errorf("failed to construct metrics endpoint: %w", err)
	}
	logsEndpoint, err := endpoint.JoinPath(baseURL, "api", "v1", "health", "logs")
	if err != nil {
		return fmt.Errorf("failed to construct logs endpoint: %w", err)
	}
	nonceEndpoint, err := endpoint.JoinPath(baseURL, "api", "v1", "health", "nonce")
	if err != nil {
		return fmt.Errorf("failed to construct nonce endpoint: %w", err)
	}

	// Store endpoints and JWT token in metadata table
	if err := storeEnrollmentConfig(baseURL.String(), enrollEndpoint, metricsEndpoint, logsEndpoint, nonceEndpoint, jwtToken, sakToken); err != nil {
		return fmt.Errorf("failed to store configuration: %w", err)
	}
	if err := performInventorySync(context.Background()); err != nil {
		fmt.Fprintf(writerFromContext(cliContext), "Post-enroll inventory sync failed: %v\n", err)
	}

	return nil
}

func storeConfigInMetadata(baseURL, enrollEndpoint, metricsEndpoint, logsEndpoint, nonceEndpoint, jwtToken, sakToken string) error {
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
	if err := pkgmetadata.SetMetadata(context.Background(), dbRW, "backend_base_url", baseURL); err != nil {
		return fmt.Errorf("failed to set backend base URL: %w", err)
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
	if err := config.SecureStateFilePermissions(stateFile); err != nil {
		return fmt.Errorf("failed to secure state database permissions: %w", err)
	}

	return nil
}

type machineInfoCollectorFunc func(context.Context) (*machineinfo.MachineInfo, error)

func (f machineInfoCollectorFunc) Collect(ctx context.Context) (*machineinfo.MachineInfo, error) {
	return f(ctx)
}

func syncInventoryOnce(ctx context.Context) error {
	state := agentstate.NewSQLite()
	sink := inventorysink.NewBackendSink(state)

	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		return fmt.Errorf("initialize nvml for inventory sync: %w", err)
	}
	defer func() { _ = nvmlInstance.Shutdown() }()

	src := inventorysource.NewMachineInfoSource(machineInfoCollectorFunc(func(context.Context) (*machineinfo.MachineInfo, error) {
		return machineinfo.GetMachineInfo(nvmlInstance)
	}))
	manager := inventory.NewManager(src, sink, 0)
	_, err = manager.CollectOnce(ctx)
	return err
}
