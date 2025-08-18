package healthexporter

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/gpuhealthconfig"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	machineinfo "github.com/leptonai/gpud/pkg/machine-info"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	// defaultRetryDelay is the default delay between retry attempts
	defaultRetryDelay = 5 * time.Second
)

var _ Exporter = (*healthExporter)(nil)

type healthExporter struct {
	ctx    context.Context
	cancel context.CancelFunc
	config *gpuhealthconfig.HealthExporterConfig

	// Data sources
	metricsStore       pkgmetrics.Store
	eventStore         eventstore.Store
	componentsRegistry components.Registry
	nvmlInstance       nvidianvml.Instance

	// HTTP client for sending data
	httpClient *http.Client

	// Last export timestamp for tracking
	lastExport time.Time
}

// New creates a new health exporter instance
func New(ctx context.Context, config *gpuhealthconfig.HealthExporterConfig, metricsStore pkgmetrics.Store, eventStore eventstore.Store, componentsRegistry components.Registry, nvmlInstance nvidianvml.Instance) Exporter {
	cctx, cancel := context.WithCancel(ctx)

	return &healthExporter{
		ctx:                cctx,
		cancel:             cancel,
		config:             config,
		metricsStore:       metricsStore,
		eventStore:         eventStore,
		componentsRegistry: componentsRegistry,
		nvmlInstance:       nvmlInstance,
		httpClient: &http.Client{
			Timeout: config.Timeout.Duration,
		},
	}
}

// Start begins the periodic export process
func (h *healthExporter) Start() error {
	if h.config.Interval.Duration <= 0 {
		log.Logger.Debug("health exporter: no interval configured, skipping")
		return nil
	}

	log.Logger.Infow("Starting health exporter")

	go func() {
		ticker := time.NewTicker(h.config.Interval.Duration)
		defer ticker.Stop()

		for {
			select {
			case <-h.ctx.Done():
				log.Logger.Infow("Context done, stopping periodic export")
				return
			case <-ticker.C:
				if err := h.exportHealthData(); err != nil {
					log.Logger.Errorw("Export failed", "error", err)
				} else {
					log.Logger.Infow("Successfully exported health data", "timestamp", time.Now())
					h.lastExport = time.Now()
				}
			}
		}
	}()

	return nil
}

// Stop gracefully shuts down the exporter
func (h *healthExporter) Stop() error {
	log.Logger.Infow("Stopping health exporter")
	h.cancel()
	return nil
}

// ExportNow triggers an immediate export
func (h *healthExporter) ExportNow(ctx context.Context) error {
	return h.exportHealthData()
}

// getMachineID gets machine ID from system (no database dependencies)
func (h *healthExporter) getMachineID(ctx context.Context) (string, error) {
	machineID := pkghost.MachineID()
	if machineID == "" {
		// Fallback to dynamic lookup if not cached
		return pkghost.GetMachineID(ctx)
	}
	return machineID, nil
}

// exportHealthData collects and exports health data
func (h *healthExporter) exportHealthData() error {
	ctx, cancel := context.WithTimeout(h.ctx, h.config.Timeout.Duration)
	defer cancel()

	// Collect health data
	healthData, err := h.collectHealthData(ctx)
	if err != nil {
		return fmt.Errorf("failed to collect health data: %w", err)
	}

	// Export data based on mode
	if h.config.OfflineMode {
		// Write to file in offline mode
		if err := h.writeHealthDataToFile(healthData); err != nil {
			return fmt.Errorf("failed to write health data to file: %w", err)
		}
	} else {
		// Send to global health endpoint in online mode
		if err := h.sendHealthData(ctx, healthData); err != nil {
			return fmt.Errorf("failed to send health data: %w", err)
		}
	}

	return nil
}

// generateCollectionID generates a unique identifier for a data collection cycle
func generateCollectionID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// collectHealthData gathers all configured health data
func (h *healthExporter) collectHealthData(ctx context.Context) (*HealthData, error) {
	collectionID := generateCollectionID()

	// Get machine ID dynamically
	machineID, err := h.getMachineID(ctx)
	if err != nil {
		log.Logger.Errorw("Failed to get machine ID", "error", err)
		return nil, fmt.Errorf("failed to get machine ID: %w", err)
	}

	data := &HealthData{
		CollectionID: collectionID,
		MachineID:    machineID,
		Timestamp:    time.Now().UTC(),
	}
	log.Logger.Infow("Starting health data collection")

	// Collect machine info if enabled
	if h.config.IncludeMachineInfo && h.nvmlInstance != nil {
		machineInfo, err := machineinfo.GetMachineInfo(h.nvmlInstance)
		if err != nil {
			log.Logger.Errorw("Failed to get machine info", "error", err)
		} else {
			data.MachineInfo = machineInfo
			log.Logger.Debugw("Collected machine info", "machine_info", data.MachineInfo)
		}
	}

	// Collect metrics if enabled
	if h.config.IncludeMetrics && h.metricsStore != nil {
		since := time.Now().Add(-h.config.MetricsLookback.Duration)
		metrics, err := h.metricsStore.Read(ctx, pkgmetrics.WithSince(since))
		if err != nil {
			log.Logger.Errorw("Failed to read metrics", "error", err)
		} else {
			data.Metrics = metrics
			log.Logger.Debugw("Collected metrics", "count", len(metrics), "metrics", metrics)
		}
	}

	// Collect events if enabled
	if h.config.IncludeEvents && h.eventStore != nil && h.componentsRegistry != nil {
		since := time.Now().Add(-h.config.EventsLookback.Duration)

		// Collect events from all components (since there's no unified "all" bucket)
		var allEvents eventstore.Events
		components := h.componentsRegistry.All()

		if len(components) == 0 {
			log.Logger.Errorw("No components found", "error", "no components found")
			return nil, fmt.Errorf("no components found")
		}

		for _, component := range components {
			componentEvents, err := component.Events(ctx, since)
			if err != nil {
				log.Logger.Errorw("Failed to get events from component", "component", component.Name(), "error", err)
				continue
			}

			if len(componentEvents) > 0 {
				// Convert apiv1.Events to eventstore.Events
				for _, event := range componentEvents {
					// Fix empty component names by using the component name we know
					componentName := event.Component
					if componentName == "" {
						componentName = component.Name()
					}

					allEvents = append(allEvents, eventstore.Event{
						Component: componentName,
						Time:      event.Time.Time,
						Name:      event.Name,
						Type:      string(event.Type),
						Message:   event.Message,
					})
				}
			}
		}

		data.Events = allEvents
		log.Logger.Debugw("Collected events", "count", len(allEvents), "events", allEvents)
	}

	// Collect component data (actual numbers/metrics) if enabled
	if h.config.IncludeComponentData && h.componentsRegistry != nil {
		componentData := h.collectComponentData()
		data.ComponentData = componentData
		log.Logger.Debugw("Collected component data", "count", len(componentData), "data", componentData)
	}

	log.Logger.Infow("Health data collection completed", "metrics", len(data.Metrics), "events", len(data.Events), "components", len(data.ComponentData))
	return data, nil
}

// collectComponentData gathers actual data/numbers from all components plus health states
func (h *healthExporter) collectComponentData() map[string]interface{} {
	componentData := make(map[string]interface{})

	components := h.componentsRegistry.All()

	for _, component := range components {
		componentName := component.Name()

		// We only need health states, not the detailed component data

		// Get health states - use the first one as primary health status
		healthStates := component.LastHealthStates()
		log.Logger.Debugw("healthexporter: Collecting health states...", "component", componentName, "health_states", healthStates)
		health := "Unknown"
		reason := "No health data"

		var timeValue interface{}
		var extraInfo interface{}

		if len(healthStates) > 0 {
			// Use the first health state as the primary status
			firstState := healthStates[0]
			health = string(firstState.Health)
			reason = firstState.Reason
			timeValue = firstState.Time
			extraInfo = firstState.ExtraInfo
		}

		// Store structure with all available fields: componentName, health, reason, time, extra_info
		componentData[componentName] = map[string]interface{}{
			"component_name": componentName,
			"health":         health,
			"reason":         reason,
			"time":           timeValue,
			"extra_info":     extraInfo,
		}
	}

	return componentData
}

// OTLPData holds both metrics and logs for OTLP export
type OTLPData struct {
	Metrics *metricsv1.MetricsData
	Logs    *logsv1.LogsData
}

// converts HealthData to OTLP metrics and logs format
func (h *healthExporter) convertToOTLP(data *HealthData) *OTLPData {
	// Create shared resource for both metrics and logs
	resource := h.createOTLPResource(data)

	// Convert metrics
	metricsData := &metricsv1.MetricsData{
		ResourceMetrics: []*metricsv1.ResourceMetrics{
			{
				Resource: resource,
				ScopeMetrics: []*metricsv1.ScopeMetrics{
					{
						Scope: &commonv1.InstrumentationScope{
							Name:    "gpuhealth-exporter",
							Version: "1.0.0",
						},
						Metrics: h.convertMetricsToOTLP(data),
					},
				},
			},
		},
	}

	// Convert logs (events + component data)
	logsData := &logsv1.LogsData{
		ResourceLogs: []*logsv1.ResourceLogs{
			{
				Resource: resource,
				ScopeLogs: []*logsv1.ScopeLogs{
					{
						Scope: &commonv1.InstrumentationScope{
							Name:    "gpuhealth-exporter",
							Version: "1.0.0",
						},
						LogRecords: h.convertToOTLPLogs(data),
					},
				},
			},
		},
	}

	return &OTLPData{
		Metrics: metricsData, // machine_info, metrics
		Logs:    logsData,    // machine_info, events, component data
	}
}

// convertStructToOTLPAttributes converts a struct to OTLP attributes using reflection
// It handles string, int64, uint64, time.Time fields, and recursively processes nested structs
func convertStructToOTLPAttributes(v interface{}) []*commonv1.KeyValue {
	return convertStructToOTLPAttributesWithPrefix(v, "")
}

// convertStructToOTLPAttributesWithPrefix converts a struct to OTLP attributes with a key prefix
func convertStructToOTLPAttributesWithPrefix(v interface{}, prefix string) []*commonv1.KeyValue {
	var attributes []*commonv1.KeyValue

	if v == nil {
		return attributes
	}

	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return attributes
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return attributes
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Skip unexported fields
		if !field.CanInterface() {
			continue
		}

		// Get JSON tag for field name, fall back to field name
		jsonTag := fieldType.Tag.Get("json")
		fieldName := fieldType.Name
		if jsonTag != "" && jsonTag != "-" {
			// Extract field name from json tag (remove omitempty, etc.)
			if commaIdx := strings.Index(jsonTag, ","); commaIdx != -1 {
				fieldName = jsonTag[:commaIdx]
			} else {
				fieldName = jsonTag
			}
		}

		// Add prefix if provided
		fullFieldName := fieldName
		if prefix != "" {
			fullFieldName = prefix + "." + fieldName
		}

		// Convert field value to string if it's not empty/nil
		var stringValue string
		switch field.Kind() {
		case reflect.String:
			stringValue = field.String()
		case reflect.Int64:
			if field.Int() != 0 {
				stringValue = fmt.Sprintf("%d", field.Int())
			}
		case reflect.Uint64:
			if field.Uint() != 0 {
				stringValue = fmt.Sprintf("%d", field.Uint())
			}
		case reflect.Struct:
			// Handle time.Time specially
			if field.Type().String() == "time.Time" {
				if timeVal, ok := field.Interface().(time.Time); ok && !timeVal.IsZero() {
					stringValue = timeVal.Format(time.RFC3339)
				}
			} else {
				// Recursively process nested structs
				nestedAttributes := convertStructToOTLPAttributesWithPrefix(field.Interface(), fullFieldName)
				attributes = append(attributes, nestedAttributes...)
				continue
			}
		case reflect.Ptr:
			// Handle pointer fields by dereferencing and processing recursively
			if !field.IsNil() {
				nestedAttributes := convertStructToOTLPAttributesWithPrefix(field.Interface(), fullFieldName)
				attributes = append(attributes, nestedAttributes...)
			}
			continue
		case reflect.Slice:
			// Handle slices (like GPUs array) by converting to JSON string
			if field.Len() > 0 {
				if jsonBytes, err := json.Marshal(field.Interface()); err == nil {
					stringValue = string(jsonBytes)
				}
			}
		default:
			// Skip other types
			continue
		}

		// Only add non-empty values
		if stringValue != "" {
			attributes = append(attributes, &commonv1.KeyValue{
				Key: fullFieldName,
				Value: &commonv1.AnyValue{
					Value: &commonv1.AnyValue_StringValue{StringValue: stringValue},
				},
			})
		}
	}

	return attributes
}

// creates OTLP resource with machine info and identification
func (h *healthExporter) createOTLPResource(data *HealthData) *resourcev1.Resource {

	attributes := []*commonv1.KeyValue{
		{
			Key: "service.name",
			Value: &commonv1.AnyValue{
				Value: &commonv1.AnyValue_StringValue{StringValue: "gpu-health-agent"},
			},
		},
		{
			Key: "machine.id",
			Value: &commonv1.AnyValue{
				Value: &commonv1.AnyValue_StringValue{StringValue: data.MachineID},
			},
		},
	}

	// Add machine info attributes if available using reflection
	if data.MachineInfo != nil {
		machineInfoAttributes := convertStructToOTLPAttributes(data.MachineInfo)
		attributes = append(attributes, machineInfoAttributes...)
	}

	return &resourcev1.Resource{
		Attributes: attributes,
	}
}

// converts gpud metrics to OTLP metrics format
func (h *healthExporter) convertMetricsToOTLP(data *HealthData) []*metricsv1.Metric {

	var otlpMetrics []*metricsv1.Metric

	// Convert regular metrics if available
	if len(data.Metrics) > 0 {
		for _, metric := range data.Metrics {
			otlpMetric := &metricsv1.Metric{
				Name:        metric.Name,
				Description: fmt.Sprintf("Metric from component %s", metric.Component),
				Unit:        "1",
				Data: &metricsv1.Metric_Gauge{
					Gauge: &metricsv1.Gauge{
						DataPoints: []*metricsv1.NumberDataPoint{
							{
								TimeUnixNano: uint64(time.Unix(0, metric.UnixMilliseconds*1_000_000).UnixNano()),
								Value: &metricsv1.NumberDataPoint_AsDouble{
									AsDouble: metric.Value,
								},
								Attributes: h.convertLabelsToOTLPAttributes(metric.Labels),
							},
						},
					},
				},
			}
			otlpMetrics = append(otlpMetrics, otlpMetric)
		}
	}

	// Add a summary metric with collection info
	summaryMetric := &metricsv1.Metric{
		Name:        "gpud_health_agent_collection_info",
		Description: "Summary information about health data collection",
		Unit:        "1",
		Data: &metricsv1.Metric_Gauge{
			Gauge: &metricsv1.Gauge{
				DataPoints: []*metricsv1.NumberDataPoint{
					{
						TimeUnixNano: uint64(data.Timestamp.UnixNano()),
						Value: &metricsv1.NumberDataPoint_AsInt{
							AsInt: 1,
						},
						Attributes: []*commonv1.KeyValue{
							{
								Key: "metrics_count",
								Value: &commonv1.AnyValue{
									Value: &commonv1.AnyValue_IntValue{IntValue: int64(len(data.Metrics))},
								},
							},
							{
								Key: "events_count",
								Value: &commonv1.AnyValue{
									Value: &commonv1.AnyValue_IntValue{IntValue: int64(len(data.Events))},
								},
							},
							{
								Key: "component_data_count",
								Value: &commonv1.AnyValue{
									Value: &commonv1.AnyValue_IntValue{IntValue: int64(len(data.ComponentData))},
								},
							},
						},
					},
				},
			},
		},
	}
	otlpMetrics = append(otlpMetrics, summaryMetric)

	return otlpMetrics
}

// convertLabelsToOTLPAttributes converts metric labels to OTLP attributes
func (h *healthExporter) convertLabelsToOTLPAttributes(labels map[string]string) []*commonv1.KeyValue {
	var attributes []*commonv1.KeyValue
	for key, value := range labels {
		attributes = append(attributes, &commonv1.KeyValue{
			Key: key,
			Value: &commonv1.AnyValue{
				Value: &commonv1.AnyValue_StringValue{StringValue: value},
			},
		})
	}
	return attributes
}

// convertToOTLPLogs converts HealthData events and component data to OTLP log records
func (h *healthExporter) convertToOTLPLogs(data *HealthData) []*logsv1.LogRecord {

	var logRecords []*logsv1.LogRecord

	// Add events as log records
	if len(data.Events) > 0 {
		for _, event := range data.Events {
			logRecord := &logsv1.LogRecord{
				TimeUnixNano:   uint64(event.Time.UnixNano()),
				SeverityNumber: logsv1.SeverityNumber_SEVERITY_NUMBER_INFO,
				SeverityText:   "INFO",
				Body: &commonv1.AnyValue{
					Value: &commonv1.AnyValue_StringValue{
						StringValue: fmt.Sprintf("[%s] %s: %s", event.Type, event.Component, event.Message),
					},
				},
				Attributes: []*commonv1.KeyValue{
					{
						Key: "component",
						Value: &commonv1.AnyValue{
							Value: &commonv1.AnyValue_StringValue{StringValue: event.Component},
						},
					},
					{
						Key: "event_name",
						Value: &commonv1.AnyValue{
							Value: &commonv1.AnyValue_StringValue{StringValue: event.Name},
						},
					},
					{
						Key: "event_type",
						Value: &commonv1.AnyValue{
							Value: &commonv1.AnyValue_StringValue{StringValue: event.Type},
						},
					},
					{
						Key: "log_type",
						Value: &commonv1.AnyValue{
							Value: &commonv1.AnyValue_StringValue{StringValue: "event"},
						},
					},
				},
			}
			logRecords = append(logRecords, logRecord)
		}
	}

	// Add component data as log records with timestamp and extra info
	if len(data.ComponentData) > 0 {

		for componentName, componentResult := range data.ComponentData {

			// Cast to simple map structure
			componentInfo, ok := componentResult.(map[string]interface{})
			if !ok {
				log.Logger.Warnw("Unexpected component data format", "component", componentName)
				continue
			}

			// Extract values from health states
			health := componentInfo["health"]
			reason := componentInfo["reason"]
			time := componentInfo["time"]
			extraInfo := componentInfo["extra_info"]

			// Create attributes with all health state information
			attributes := []*commonv1.KeyValue{
				{
					Key: "component",
					Value: &commonv1.AnyValue{
						Value: &commonv1.AnyValue_StringValue{StringValue: componentName},
					},
				},
				{
					Key: "log_type",
					Value: &commonv1.AnyValue{
						Value: &commonv1.AnyValue_StringValue{StringValue: "component_data"},
					},
				},
				{
					Key: "health",
					Value: &commonv1.AnyValue{
						Value: &commonv1.AnyValue_StringValue{StringValue: fmt.Sprintf("%v", health)},
					},
				},
				{
					Key: "reason",
					Value: &commonv1.AnyValue{
						Value: &commonv1.AnyValue_StringValue{StringValue: fmt.Sprintf("%v", reason)},
					},
				},
			}

			// Add time if available
			if time != nil {
				attributes = append(attributes, &commonv1.KeyValue{
					Key: "time",
					Value: &commonv1.AnyValue{
						Value: &commonv1.AnyValue_StringValue{StringValue: fmt.Sprintf("%v", time)},
					},
				})
			}

			// Add extra_info if available
			if extraInfo != nil {
				attributes = append(attributes, &commonv1.KeyValue{
					Key: "extra_info",
					Value: &commonv1.AnyValue{
						Value: &commonv1.AnyValue_StringValue{StringValue: fmt.Sprintf("%v", extraInfo)},
					},
				})
			}

			logRecord := &logsv1.LogRecord{
				TimeUnixNano:   uint64(data.Timestamp.UnixNano()),
				SeverityNumber: logsv1.SeverityNumber_SEVERITY_NUMBER_INFO,
				SeverityText:   "INFO",
				Body: &commonv1.AnyValue{
					Value: &commonv1.AnyValue_StringValue{
						StringValue: fmt.Sprintf("Component [%s]: %v - %v", componentName, health, reason),
					},
				},
				Attributes: attributes,
			}
			logRecords = append(logRecords, logRecord)
		}
	}

	return logRecords
}

// sendHealthData sends the collected data to the global health endpoint using OTLP format
// TODO: limit the size of the data to be sent
// TODO: add a CLI option to send the data to a local file
func (h *healthExporter) sendHealthData(ctx context.Context, data *HealthData) error {

	// Convert to OTLP format
	otlpRequest := h.convertToOTLP(data)

	// Send combined data (metrics + logs) as separate requests
	// Send metrics first
	if otlpRequest.Metrics != nil && len(otlpRequest.Metrics.ResourceMetrics) > 0 {
		metricsBytes, err := proto.Marshal(otlpRequest.Metrics)
		if err != nil {
			log.Logger.Errorw("Failed to marshal OTLP metrics data", "error", err)
			return err
		}

		// Send metrics request with retry
		if err := h.sendOTLPRequestWithRetry(ctx, metricsBytes, "metrics", data.CollectionID); err != nil {
			log.Logger.Errorw("healthexporter: CRITICAL - Failed to send metrics data after all retries",
				"collection_id", data.CollectionID,
				"error", err,
				"size_bytes", len(metricsBytes),
				"will_continue_with_logs", true)
			// Continue to send logs even if metrics fail - this ensures we don't lose event data
		}

	}

	// Send logs
	if otlpRequest.Logs != nil && len(otlpRequest.Logs.ResourceLogs) > 0 {
		logsBytes, err := proto.Marshal(otlpRequest.Logs)
		if err != nil {
			log.Logger.Errorw("Failed to marshal OTLP logs data", "error", err)
			return err
		}

		// Send logs request with retry
		if err := h.sendOTLPRequestWithRetry(ctx, logsBytes, "logs", data.CollectionID); err != nil {
			log.Logger.Errorw("healthexporter: CRITICAL - Failed to send logs data after all retries",
				"collection_id", data.CollectionID,
				"error", err,
				"size_bytes", len(logsBytes),
				"contains_events", true)
			return fmt.Errorf("failed to send critical logs data (includes events): %w", err)
		}
	}

	log.Logger.Infow("Successfully sent health data to endpoint")
	return nil
}

// writeHealthDataToFile writes standard OTLP JSON files for direct curl usage
func (h *healthExporter) writeHealthDataToFile(data *HealthData) error {
	timestamp := data.Timestamp.Format("20060102_150405")
	otlpRequest := h.convertToOTLP(data)

	// Write pure OTLP JSON files for direct use with OTEL collectors via curl
	if otlpRequest.Metrics != nil {
		metricsFilename := filepath.Join(h.config.OutputPath, fmt.Sprintf("gpuhealth_metrics_%s.json", timestamp))
		if err := h.writeOTLPJSONFile(metricsFilename, otlpRequest.Metrics); err != nil {
			return fmt.Errorf("failed to write OTLP metrics file: %w", err)
		}
	}

	if otlpRequest.Logs != nil {
		logsFilename := filepath.Join(h.config.OutputPath, fmt.Sprintf("gpuhealth_logs_%s.json", timestamp))
		if err := h.writeOTLPJSONFile(logsFilename, otlpRequest.Logs); err != nil {
			return fmt.Errorf("failed to write OTLP logs file: %w", err)
		}
	}

	log.Logger.Infow("Successfully wrote health data files", "path", h.config.OutputPath)
	return nil
}

// writeOTLPJSONFile writes protobuf message as standard OTLP JSON format
func (h *healthExporter) writeOTLPJSONFile(filename string, message proto.Message) error {
	// Use protojson to ensure proper OTLP field naming and format
	marshaler := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		UseProtoNames:   false, // Use JSON field names (camelCase)
		EmitUnpopulated: false, // Don't emit empty fields
	}

	jsonData, err := marshaler.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal OTLP message to JSON: %w", err)
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", filename, err)
	}
	defer file.Close()

	if _, err := file.Write(jsonData); err != nil {
		return fmt.Errorf("failed to write OTLP JSON to file %s: %w", filename, err)
	}

	return nil
}

// sendOTLPRequestWithRetry sends the OTLP data with retry logic
func (h *healthExporter) sendOTLPRequestWithRetry(ctx context.Context, reqData []byte, dataType string, collectionID string) error {
	maxAttempts := h.config.RetryMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1 // At least one attempt
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Try to send the request
		err := h.sendOTLPRequest(ctx, reqData, dataType, collectionID)
		if err == nil {
			// Success!
			if attempt > 1 {
				log.Logger.Infow("healthexporter: Request succeeded after retries",
					"data_type", dataType,
					"collection_id", collectionID,
					"attempt", attempt,
					"total_attempts", maxAttempts)
			}
			return nil
		}

		lastErr = err

		// If this was the last attempt, don't wait
		if attempt >= maxAttempts {
			break
		}

		// Simple exponential backoff: 2s, 4s, 8s, capped at 10s
		delay := defaultRetryDelay

		log.Logger.Warnw("healthexporter: Request failed, retrying",
			"data_type", dataType,
			"collection_id", collectionID,
			"attempt", attempt,
			"total_attempts", maxAttempts,
			"delay_seconds", delay.Seconds(),
			"error", err)

		// Wait before retrying (with context cancellation support)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	// All retries failed
	log.Logger.Errorw("healthexporter: All retry attempts failed",
		"data_type", dataType,
		"collection_id", collectionID,
		"total_attempts", maxAttempts,
		"final_error", lastErr)

	return fmt.Errorf("failed after %d attempts: %w", maxAttempts, lastErr)
}

// sends a single OTLP request
func (h *healthExporter) sendOTLPRequest(ctx context.Context, reqData []byte, dataType string, collectionID string) error {
	contentType := "application/x-protobuf"

	// Get machine ID dynamically
	machineID, err := h.getMachineID(ctx)
	if err != nil {
		log.Logger.Errorw("Failed to get machine ID for HTTP header", "type", dataType, "error", err)
		return fmt.Errorf("failed to get machine ID: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", h.config.Endpoint, bytes.NewBuffer(reqData))
	if err != nil {
		log.Logger.Errorw("Failed to create HTTP request", "type", dataType, "error", err)
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "gpuhealth-exporter")
	req.Header.Set("X-Machine-ID", machineID)
	req.Header.Set("X-Data-Type", dataType)         // Add data type header to help receiver
	req.Header.Set("X-Collection-ID", collectionID) // Add collection ID to correlate logs and metrics

	// Send request
	resp, err := h.httpClient.Do(req)
	if err != nil {
		log.Logger.Errorw("Failed to send HTTP request", "type", dataType, "error", err)
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Logger.Errorw("HTTP request failed", "type", dataType, "status", resp.StatusCode, "status_text", resp.Status)
		return fmt.Errorf("HTTP request failed: %s (status %d)", resp.Status, resp.StatusCode)
	}

	return nil
}
