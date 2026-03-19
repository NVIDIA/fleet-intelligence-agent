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

// Package healthexporter provides functionality to export health data from local SQLite
// to a global health endpoint for centralized monitoring and long-term storage using OTLP format.
//
// This package follows Go best practices with separated concerns:
// - collector: Data collection from various sources
// - converter: Conversion to different formats (OTLP, CSV)
// - writer: Output to files or HTTP endpoints
package exporter

import (
	"context"
	"fmt"
	"time"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	pkgmetadata "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metadata"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/attestation"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/enrollment"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/exporter/collector"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/exporter/converter"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/exporter/writer"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/registry"
)

// Ensure healthExporter implements the Exporter interface
var _ Exporter = (*healthExporter)(nil)

// healthExporter implements the Exporter interface with improved architecture
type healthExporter struct {
	ctx                context.Context
	cancel             context.CancelFunc
	options            *exporterOptions
	collector          collector.Collector
	fileWriter         writer.FileWriter
	httpWriter         writer.HTTPWriter
	attestationManager *attestation.Manager

	// Last export timestamp for tracking
	lastExport time.Time
}

// New creates a new health exporter instance using functional options
func New(ctx context.Context, opts ...ExporterOption) (Exporter, error) {
	cctx, cancel := context.WithCancel(ctx)

	// Apply options
	options := &exporterOptions{}
	for _, opt := range opts {
		if err := opt(options); err != nil {
			cancel()
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	// Validate and set defaults
	if err := options.validate(); err != nil {
		cancel()
		return nil, fmt.Errorf("invalid options: %w", err)
	}
	options.setDefaults()

	// Create attestation manager (always enabled)
	attestationManager := attestation.NewManager(cctx, options.nvmlInstance, &options.config.Attestation)
	log.Logger.Infow("Attestation manager created", "interval", options.config.Attestation.Interval.Duration, "jitter_enabled", options.config.Attestation.JitterEnabled)

	// Get all component names for config export
	allComponentNames := registry.AllComponentNames()

	dataCollector := collector.New(
		options.config,
		options.fullConfig,
		allComponentNames,
		options.metricsStore,
		options.eventStore,
		options.componentsRegistry,
		options.nvmlInstance,
		attestationManager,
		options.machineID,
		options.dcgmGPUIndexes,
	)

	otlpConverter := converter.NewOTLPConverter()
	csvConverter := converter.NewCSVConverter()

	fileWriter := writer.NewFileWriter(otlpConverter, csvConverter)
	httpWriter := writer.NewHTTPWriter(options.httpClient, otlpConverter)

	exporter := &healthExporter{
		ctx:                cctx,
		cancel:             cancel,
		options:            options,
		collector:          dataCollector,
		fileWriter:         fileWriter,
		httpWriter:         httpWriter,
		attestationManager: attestationManager,
	}

	// Set JWT refresh function on the HTTP writer
	httpWriter.SetJWTRefreshFunc(exporter.refreshJWTToken)

	return exporter, nil
}

// Start begins the periodic export process
func (e *healthExporter) Start() error {
	if e.options.config.Interval.Duration <= 0 {
		log.Logger.Debug("health exporter: no interval configured, skipping")
		return nil
	}

	log.Logger.Infow("Starting health exporter")

	// Start the attestation manager if enabled
	if e.attestationManager != nil {
		e.attestationManager.Start()
	}

	// Start the health export ticker
	go func() {
		ticker := time.NewTicker(e.options.config.Interval.Duration)
		defer ticker.Stop()

		for {
			select {
			case <-e.ctx.Done():
				log.Logger.Infow("Context done, stopping periodic export")
				return
			case <-ticker.C:
				if err := e.export(); err != nil {
					log.Logger.Errorw("Export failed", "error", err)
				} else {
					e.lastExport = time.Now().UTC()
				}
			}
		}
	}()

	return nil
}

// Stop gracefully shuts down the exporter
func (e *healthExporter) Stop() error {
	log.Logger.Infow("Stopping health exporter")
	if e.attestationManager != nil {
		e.attestationManager.Stop()
	}
	e.cancel()
	return nil
}

// ExportNow triggers an immediate export
func (e *healthExporter) ExportNow(ctx context.Context) error {
	return e.export()
}

// export performs the actual data export operation
func (e *healthExporter) export() error {
	log.Logger.Infow("Starting health export")
	ctx, cancel := context.WithTimeout(e.ctx, e.options.timeout)
	defer cancel()

	// Refresh configuration from metadata on every export
	// If the endpoints/auth token are not empty, export will continue
	// If the endpoints/auth token are empty, exportHTTP will skip
	e.refreshConfigFromMetadata(ctx)

	// Collect health data
	healthData, err := e.collector.Collect(ctx)
	if err != nil {
		return fmt.Errorf("collection failed: %w", err)
	}

	// Export data based on mode
	if e.options.config.OfflineMode {
		return e.exportToFile(healthData)
	} else {
		return e.exportToHTTP(ctx, healthData)
	}
}

// exportToFile writes health data to files
func (e *healthExporter) exportToFile(data *collector.HealthData) error {
	outputFormat := e.options.config.OutputFormat
	if outputFormat == "" {
		outputFormat = "json"
	}

	switch outputFormat {
	case "csv":
		if err := e.fileWriter.WriteCSV(data, e.options.config.OutputPath); err != nil {
			return fmt.Errorf("failed to write CSV file: %w", err)
		}
	case "json":
		if err := e.fileWriter.WriteJSON(data, e.options.config.OutputPath); err != nil {
			return fmt.Errorf("failed to write JSON file: %w", err)
		}
	default:
		return fmt.Errorf("unsupported output format: %s", outputFormat)
	}

	// Log the timestamp of the export on successful export
	log.Logger.Infow("Successfully exported health data to file", "timestamp", time.Now().UTC())
	return nil
}

// exportToHTTP sends health data via HTTP
func (e *healthExporter) exportToHTTP(ctx context.Context, data *collector.HealthData) error {
	// Skip export if no endpoints are configured
	if e.options.config.MetricsEndpoint == "" && e.options.config.LogsEndpoint == "" {
		log.Logger.Infow("No endpoints configured, skipping HTTP export")
		return nil
	}

	if e.options.config.AuthToken == "" {
		log.Logger.Infow("No auth token configured, skipping HTTP export")
		return nil
	}

	newToken, err := e.httpWriter.Send(ctx, data, e.options.config.MetricsEndpoint, e.options.config.LogsEndpoint, e.options.config.RetryMaxAttempts, e.options.config.AuthToken, e.options.config.Interval.Duration)
	if err != nil {
		return fmt.Errorf("failed to send data: %w", err)
	}

	// Log the timestamp of the export on successful export - this won't be logged if not enrolled
	log.Logger.Infow("Successfully exported health data to", "timestamp", time.Now().UTC())

	// If we received a new JWT token, update it in metadata and config
	if newToken != "" && newToken != e.options.config.AuthToken {
		log.Logger.Info("Updating JWT token from server response")

		if err := e.updateTokenInMetadata(ctx, newToken); err != nil {
			log.Logger.Errorw("Failed to update JWT token in metadata", "error", err)
			// Don't fail the export if token update fails
		} else {
			e.options.config.AuthToken = newToken
			log.Logger.Infow("Successfully updated JWT token")
		}
	}

	return nil
}

// refreshConfigFromMetadata updates the exporter configuration from metadata table
func (e *healthExporter) refreshConfigFromMetadata(ctx context.Context) {
	// Use the passed database connection instead of opening a new one
	if e.options.dbRO == nil {
		log.Logger.Debugw("no database connection available for metadata refresh")
		return
	}

	// Load metrics endpoint (update even if empty to handle un-enrollment)
	if metricsEndpoint, err := pkgmetadata.ReadMetadata(ctx, e.options.dbRO, "metrics_endpoint"); err == nil {
		if e.options.config.MetricsEndpoint != metricsEndpoint {
			e.options.config.MetricsEndpoint = metricsEndpoint
			if metricsEndpoint == "" {
				log.Logger.Infow("cleared metrics endpoint from metadata")
			} else {
				log.Logger.Infow("updated metrics endpoint from metadata", "metrics_endpoint", metricsEndpoint)
			}
		}
	} else {
		log.Logger.Errorw("failed to read metrics endpoint from metadata", "error", err)
	}

	// Load logs endpoint (update even if empty to handle un-enrollment)
	if logsEndpoint, err := pkgmetadata.ReadMetadata(ctx, e.options.dbRO, "logs_endpoint"); err == nil {
		if e.options.config.LogsEndpoint != logsEndpoint {
			e.options.config.LogsEndpoint = logsEndpoint
			if logsEndpoint == "" {
				log.Logger.Infow("cleared logs endpoint from metadata")
			} else {
				log.Logger.Infow("updated logs endpoint from metadata", "logs_endpoint", logsEndpoint)
			}
		}
	} else {
		log.Logger.Errorw("failed to read logs endpoint from metadata", "error", err)
	}

	// Load auth token (update even if empty to handle un-enrollment)
	if token, err := pkgmetadata.ReadMetadata(ctx, e.options.dbRO, pkgmetadata.MetadataKeyToken); err == nil {
		if e.options.config.AuthToken != token {
			e.options.config.AuthToken = token
			if token == "" {
				log.Logger.Infow("cleared auth token from metadata")
			} else {
				log.Logger.Infow("updated auth token from metadata")
			}
		}
	} else {
		log.Logger.Errorw("failed to read auth token from metadata", "error", err)
	}
}

// updateTokenInMetadata updates the JWT token in the metadata database
func (e *healthExporter) updateTokenInMetadata(ctx context.Context, newToken string) error {
	// Use the passed database connection instead of opening a new one
	if e.options.dbRW == nil {
		return fmt.Errorf("no read-write database connection available for token update")
	}

	if err := pkgmetadata.SetMetadata(ctx, e.options.dbRW, pkgmetadata.MetadataKeyToken, newToken); err != nil {
		return fmt.Errorf("failed to update JWT token in metadata: %w", err)
	}

	return nil
}

// refreshJWTToken attempts to get a new JWT token using the stored SAK token
func (e *healthExporter) refreshJWTToken(ctx context.Context) (string, error) {
	// Use the passed database connection to read SAK token and endpoints
	if e.options.dbRO == nil {
		return "", fmt.Errorf("no database connection available for JWT refresh")
	}

	// Read SAK token from metadata
	sakToken, err := pkgmetadata.ReadMetadata(ctx, e.options.dbRO, "sak_token")
	if err != nil || sakToken == "" {
		return "", fmt.Errorf("no SAK token available for JWT refresh")
	}

	// Read enroll endpoint from metadata (stored during enrollment)
	enrollEndpoint, err := pkgmetadata.ReadMetadata(ctx, e.options.dbRO, "enroll_endpoint")
	if err != nil || enrollEndpoint == "" {
		return "", fmt.Errorf("no enroll endpoint available for JWT refresh")
	}

	// Perform enrollment to get new JWT token using the shared function
	newJWT, err := enrollment.PerformEnrollment(ctx, enrollEndpoint, sakToken)
	if err != nil {
		return "", fmt.Errorf("failed to refresh JWT token: %w", err)
	}

	// Update JWT token in metadata using the read-write connection
	if e.options.dbRW != nil {
		if err := pkgmetadata.SetMetadata(ctx, e.options.dbRW, pkgmetadata.MetadataKeyToken, newJWT); err != nil {
			log.Logger.Errorw("Failed to update refreshed JWT token in metadata", "error", err)
			// Don't fail the refresh, just log the error
		}
	}

	log.Logger.Infow("Successfully refreshed JWT token")
	return newJWT, nil
}
