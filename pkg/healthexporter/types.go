// Package healthexporter provides functionality to export health data from local SQLite
// to a global health endpoint for centralized monitoring and long-term storage using OTLP format.
package healthexporter

import (
	"context"
)

// Exporter defines the interface for health data export functionality
type Exporter interface {
	// Start begins the periodic export process
	Start() error

	// Stop gracefully shuts down the exporter
	Stop() error

	// ExportNow triggers an immediate export (for testing/manual use)
	ExportNow(ctx context.Context) error
}
