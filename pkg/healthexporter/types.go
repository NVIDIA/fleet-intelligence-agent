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
	MachineID   string
	Timestamp   time.Time
	
	// Machine hardware info (from gossip)
	MachineInfo *apiv1.MachineInfo
	
	// Metrics data from SQLite
	Metrics pkgmetrics.Metrics
	
	// Events data from SQLite  
	Events eventstore.Events
	
	// Component actual data/numbers (raw check results)
	ComponentData map[string]interface{}
}

// Config holds configuration for the health exporter
type Config struct {
	// Endpoint is the global health endpoint URL
	Endpoint string
	
	// Interval is how often to export data
	Interval time.Duration
	
	// MachineID identifies this machine
	MachineID string
	
	// Timeout for HTTP requests
	Timeout time.Duration
	
	// IncludeMetrics controls whether to include metrics data
	IncludeMetrics bool
	
	// IncludeEvents controls whether to include events data
	IncludeEvents bool
	
	// IncludeMachineInfo controls whether to include machine hardware info
	IncludeMachineInfo bool
	
	// IncludeComponentData controls whether to include actual component data/numbers
	IncludeComponentData bool
	
	// MetricsLookback determines how far back to look for metrics data
	MetricsLookback time.Duration
	
	// EventsLookback determines how far back to look for events data
	EventsLookback time.Duration
	
	// RetryMaxAttempts is the maximum number of retry attempts for failed requests
	RetryMaxAttempts int
}

// DefaultConfig returns a sensible default configuration for OTLP format
func DefaultConfig() *Config {
	return &Config{
		Endpoint:             "http://localhost:8080/api/v1/health/bulk",
		Interval:             5 * time.Minute,
		Timeout:              30 * time.Second,
		IncludeMetrics:       true,
		IncludeEvents:        true,
		IncludeMachineInfo:   true,
		IncludeComponentData: true,
		MetricsLookback:      15 * time.Minute, // Get metrics from last 15 minutes
		EventsLookback:       1 * time.Hour,    // Get events from last hour
		RetryMaxAttempts:     3,
	}
} 