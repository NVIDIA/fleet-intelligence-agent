package healthexporter

import (
	"errors"
	"net/http"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/gpuhealthconfig"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

// ExporterOption defines a function that configures the health exporter
// Note: For configuration values, use gpuhealthconfig.HealthExporterConfig only.
// This options struct is strictly for wiring dependencies and runtime options.
type ExporterOption func(*exporterOptions) error

// exporterOptions holds dependencies and runtime options for the health exporter.
// Configuration values should be sourced from gpuhealthconfig.HealthExporterConfig.
type exporterOptions struct {
	config             *gpuhealthconfig.HealthExporterConfig
	metricsStore       pkgmetrics.Store
	eventStore         eventstore.Store
	componentsRegistry components.Registry
	nvmlInstance       nvidianvml.Instance
	httpClient         *http.Client
	timeout            time.Duration
}

// WithConfig sets the health exporter configuration
func WithConfig(config *gpuhealthconfig.HealthExporterConfig) ExporterOption {
	return func(c *exporterOptions) error {
		if config == nil {
			return errors.New("configuration cannot be nil")
		}
		c.config = config
		c.timeout = config.Timeout.Duration
		return nil
	}
}

// WithMetricsStore sets the metrics store
func WithMetricsStore(store pkgmetrics.Store) ExporterOption {
	return func(c *exporterOptions) error {
		c.metricsStore = store
		return nil
	}
}

// WithEventStore sets the event store
func WithEventStore(store eventstore.Store) ExporterOption {
	return func(c *exporterOptions) error {
		c.eventStore = store
		return nil
	}
}

// WithComponentsRegistry sets the components registry
func WithComponentsRegistry(registry components.Registry) ExporterOption {
	return func(c *exporterOptions) error {
		c.componentsRegistry = registry
		return nil
	}
}

// WithNVMLInstance sets the NVML instance
func WithNVMLInstance(instance nvidianvml.Instance) ExporterOption {
	return func(c *exporterOptions) error {
		c.nvmlInstance = instance
		return nil
	}
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(client *http.Client) ExporterOption {
	return func(c *exporterOptions) error {
		if client == nil {
			return errors.New("HTTP client cannot be nil")
		}
		c.httpClient = client
		return nil
	}
}

// WithTimeout sets a custom timeout (overrides config timeout)
func WithTimeout(timeout time.Duration) ExporterOption {
	return func(c *exporterOptions) error {
		if timeout <= 0 {
			return errors.New("timeout must be positive")
		}
		c.timeout = timeout
		return nil
	}
}

// validateConfig validates the exporter options and required dependencies
func (c *exporterOptions) validate() error {
	if c.config == nil {
		return errors.New("configuration is required")
	}

	// Only validate dependencies that are actually needed based on config
	if c.config.IncludeMetrics && c.metricsStore == nil {
		return errors.New("metrics store is required when IncludeMetrics is enabled")
	}

	if c.config.IncludeEvents && c.eventStore == nil {
		return errors.New("event store is required when IncludeEvents is enabled")
	}

	if c.config.IncludeComponentData && c.componentsRegistry == nil {
		return errors.New("components registry is required when IncludeComponentData is enabled")
	}

	if c.config.IncludeMachineInfo && c.nvmlInstance == nil {
		return errors.New("NVML instance is required when IncludeMachineInfo is enabled")
	}

	return nil
}

// setDefaults sets default values for unspecified options
func (c *exporterOptions) setDefaults() {
	if c.httpClient == nil && c.timeout > 0 {
		c.httpClient = &http.Client{
			Timeout: c.timeout,
		}
	}
}
