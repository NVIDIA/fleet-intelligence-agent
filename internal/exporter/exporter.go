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

	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/sqlite"

	"github.com/NVIDIA/gpuhealth/internal/attestation"
	"github.com/NVIDIA/gpuhealth/internal/config"
	"github.com/NVIDIA/gpuhealth/internal/exporter/collector"
	"github.com/NVIDIA/gpuhealth/internal/exporter/converter"
	"github.com/NVIDIA/gpuhealth/internal/exporter/writer"
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

	// Create attestation manager if enabled
	var attestationManager *attestation.Manager
	if options.config.AttestationEnabled {
		attestationManager = attestation.NewManager(cctx, options.nvmlInstance, options.config.AttestationInterval.Duration)
		log.Logger.Infow("Attestation manager created", "interval", options.config.AttestationInterval.Duration)
	} else {
		log.Logger.Infow("Attestation disabled, skipping attestation manager creation")
	}

	// Create components
	dataCollector := collector.New(
		options.config,
		options.metricsStore,
		options.eventStore,
		options.componentsRegistry,
		options.nvmlInstance,
		attestationManager,
	)

	otlpConverter := converter.NewOTLPConverter()
	csvConverter := converter.NewCSVConverter()

	fileWriter := writer.NewFileWriter(otlpConverter, csvConverter)
	httpWriter := writer.NewHTTPWriter(options.httpClient, otlpConverter)

	return &healthExporter{
		ctx:                cctx,
		cancel:             cancel,
		options:            options,
		collector:          dataCollector,
		fileWriter:         fileWriter,
		httpWriter:         httpWriter,
		attestationManager: attestationManager,
	}, nil
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
					log.Logger.Infow("Successfully exported health data", "timestamp", time.Now().UTC())
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

	if err := e.httpWriter.Send(ctx, data, e.options.config.MetricsEndpoint, e.options.config.LogsEndpoint, e.options.config.RetryMaxAttempts, e.options.config.AuthToken); err != nil {
		return fmt.Errorf("failed to send data: %w", err)
	}
	return nil
}

// refreshConfigFromMetadata updates the exporter configuration from metadata table
func (e *healthExporter) refreshConfigFromMetadata(ctx context.Context) {
	stateFile, err := config.DefaultStateFile()
	if err != nil {
		log.Logger.Debugw("failed to get state file path", "error", err)
		return
	}

	dbRO, err := sqlite.Open(stateFile)
	if err != nil {
		log.Logger.Debugw("failed to open state database", "error", err)
		return
	}
	defer dbRO.Close()

	// Load metrics endpoint
	if metricsEndpoint, err := pkgmetadata.ReadMetadata(ctx, dbRO, "metrics_endpoint"); err == nil && metricsEndpoint != "" {
		if e.options.config.MetricsEndpoint != metricsEndpoint {
			e.options.config.MetricsEndpoint = metricsEndpoint
			log.Logger.Infow("updated metrics endpoint from metadata", "metrics_endpoint", metricsEndpoint)
		}
	}

	// Load logs endpoint
	if logsEndpoint, err := pkgmetadata.ReadMetadata(ctx, dbRO, "logs_endpoint"); err == nil && logsEndpoint != "" {
		if e.options.config.LogsEndpoint != logsEndpoint {
			e.options.config.LogsEndpoint = logsEndpoint
			log.Logger.Infow("updated logs endpoint from metadata", "logs_endpoint", logsEndpoint)
		}
	}

	// Load auth token
	if token, err := pkgmetadata.ReadMetadata(ctx, dbRO, pkgmetadata.MetadataKeyToken); err == nil && token != "" {
		if e.options.config.AuthToken != token {
			e.options.config.AuthToken = token
			log.Logger.Infow("updated auth token from metadata")
		}
	}
}
