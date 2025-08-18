// Package healthexporter provides functionality to export health data from local SQLite
// to a global health endpoint for centralized monitoring and long-term storage using OTLP format.
package healthexporter

import (
	"context"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
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

// HealthData represents the combined health data to export in OTLP format
type HealthData struct {
	// Unique identifier for this specific data collection cycle
	// Used to correlate logs and metrics requests that belong to the same collection
	CollectionID string

	// Machine identification
	MachineID string
	Timestamp time.Time

	// Machine hardware info (from gossip)
	MachineInfo *apiv1.MachineInfo

	// Metrics data from SQLite
	Metrics pkgmetrics.Metrics

	// Events data from SQLite
	Events eventstore.Events

	// Component actual data/numbers (raw check results)
	ComponentData map[string]interface{}
}
