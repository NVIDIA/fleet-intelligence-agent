// Package healthexporter provides functionality to export health data from local SQLite
// to a global health endpoint for centralized monitoring and long-term storage using OTLP format.
//
// This package follows Go best practices with separated concerns:
// - collector: Data collection from various sources
// - converter: Conversion to different formats (OTLP, CSV)
// - writer: Output to files or HTTP endpoints
package healthexporter

import (
	"context"
	"fmt"
	"time"

	"github.com/leptonai/gpud/pkg/healthexporter/collector"
	"github.com/leptonai/gpud/pkg/healthexporter/converter"
	"github.com/leptonai/gpud/pkg/healthexporter/writer"
	"github.com/leptonai/gpud/pkg/log"
)

// Ensure healthExporter implements the Exporter interface
var _ Exporter = (*healthExporter)(nil)

// healthExporter implements the Exporter interface with improved architecture
type healthExporter struct {
	ctx        context.Context
	cancel     context.CancelFunc
	options    *exporterOptions
	collector  collector.Collector
	fileWriter writer.FileWriter
	httpWriter writer.HTTPWriter

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

	// Create components
	dataCollector := collector.New(
		options.config,
		options.metricsStore,
		options.eventStore,
		options.componentsRegistry,
		options.nvmlInstance,
	)

	otlpConverter := converter.NewOTLPConverter()
	csvConverter := converter.NewCSVConverter()

	fileWriter := writer.NewFileWriter(otlpConverter, csvConverter)
	httpWriter := writer.NewHTTPWriter(options.httpClient, otlpConverter)

	return &healthExporter{
		ctx:        cctx,
		cancel:     cancel,
		options:    options,
		collector:  dataCollector,
		fileWriter: fileWriter,
		httpWriter: httpWriter,
	}, nil
}

// Start begins the periodic export process
func (e *healthExporter) Start() error {
	if e.options.config.Interval.Duration <= 0 {
		log.Logger.Debug("health exporter: no interval configured, skipping")
		return nil
	}

	log.Logger.Infow("Starting health exporter")

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
					log.Logger.Infow("Successfully exported health data", "timestamp", time.Now())
					e.lastExport = time.Now()
				}
			}
		}
	}()

	return nil
}

// Stop gracefully shuts down the exporter
func (e *healthExporter) Stop() error {
	log.Logger.Infow("Stopping health exporter")
	e.cancel()
	return nil
}

// ExportNow triggers an immediate export
func (e *healthExporter) ExportNow(ctx context.Context) error {
	return e.export()
}

// export performs the actual data export operation
func (e *healthExporter) export() error {
	ctx, cancel := context.WithTimeout(e.ctx, e.options.timeout)
	defer cancel()

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
	if err := e.httpWriter.Send(ctx, data, e.options.config.Endpoint, e.options.config.RetryMaxAttempts); err != nil {
		return fmt.Errorf("failed to send data to %s: %w", e.options.config.Endpoint, err)
	}
	return nil
}
