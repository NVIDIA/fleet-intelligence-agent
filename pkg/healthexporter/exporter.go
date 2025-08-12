package healthexporter

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/machine-info"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
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
	config *Config
	
	// Data sources
	metricsStore      pkgmetrics.Store
	eventStore        eventstore.Store
	componentsRegistry components.Registry
	nvmlInstance      nvidianvml.Instance
	
	// HTTP client for sending data
	httpClient *http.Client
	
	// Last export timestamp for tracking
	lastExport time.Time
}

// New creates a new health exporter instance
func New(ctx context.Context, config *Config, metricsStore pkgmetrics.Store, eventStore eventstore.Store, componentsRegistry components.Registry, nvmlInstance nvidianvml.Instance) Exporter {
	cctx, cancel := context.WithCancel(ctx)
	
	return &healthExporter{
		ctx:               cctx,
		cancel:            cancel,
		config:            config,
		metricsStore:      metricsStore,
		eventStore:        eventStore,
		componentsRegistry: componentsRegistry,
		nvmlInstance:      nvmlInstance,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// Start begins the periodic export process
func (h *healthExporter) Start() error {
	if h.config.Interval <= 0 {
		log.Logger.Debug("health exporter: no interval configured, skipping")
		return nil
	}
	
	log.Logger.Infow("Starting health exporter")
	
	go func() {
		ticker := time.NewTicker(h.config.Interval)
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

// exportHealthData collects and exports health data
func (h *healthExporter) exportHealthData() error {
	log.Logger.Infow("healthexporter: Health Exporter: Exporting health data")
	ctx, cancel := context.WithTimeout(h.ctx, h.config.Timeout)
	defer cancel()
	
	// Collect health data
	healthData, err := h.collectHealthData(ctx)
	if err != nil {
		return fmt.Errorf("failed to collect health data: %w", err)
	}
	
	// Send to global health endpoint
	if err := h.sendHealthData(ctx, healthData); err != nil {
		return fmt.Errorf("failed to send health data: %w", err)
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
	data := &HealthData{
		CollectionID: collectionID,
		MachineID:    h.config.MachineID,
		Timestamp:    time.Now().UTC(),
	}
	log.Logger.Infow("healthexporter: Starting data collection...", "collection_id", collectionID, "timestamp", data.Timestamp.Format("2006-01-02 15:04:05 PDT"), "machine_id", h.config.MachineID)
	
	// Collect machine info if enabled
	if h.config.IncludeMachineInfo && h.nvmlInstance != nil {
		log.Logger.Infow("healthexporter: Collecting machine info...")
		machineInfo, err := machineinfo.GetMachineInfo(h.nvmlInstance)
		if err != nil {
			log.Logger.Errorw("Failed to get machine info", "error", err)
		} else {
			data.MachineInfo = machineInfo
			log.Logger.Infow("healthexporter: Collected machine info", "machine_info", data.MachineInfo)
		}
	} else {
		log.Logger.Infow("Machine info collection disabled")
	}
	log.Logger.Infow("healthexporter: Machine info collection completed", "machine_info", data.MachineInfo)
	
	// Collect metrics if enabled
	if h.config.IncludeMetrics && h.metricsStore != nil {
		since := time.Now().Add(-h.config.MetricsLookback)
		log.Logger.Infow("healthexporter: Collecting metrics...", "since", since.Format("15:04:05 PDT"))
		
		metrics, err := h.metricsStore.Read(ctx, pkgmetrics.WithSince(since))
		if err != nil {
			log.Logger.Errorw("Failed to read metrics", "error", err)
		} else {
			data.Metrics = metrics
			log.Logger.Infow("healthexporter: Collected metrics", "count", len(metrics), "metrics", metrics)
			
			// Show sample metrics
			if len(metrics) > 0 {
				for i, metric := range metrics {
					if i >= 10 { // Show max 3 samples
						log.Logger.Infow("healthexporter: ... and more", "count", len(metrics)-10)
						break
					}
					log.Logger.Infow("healthexporter: Metric", "metric", metric)
				}
			}
		}
	} else {
		log.Logger.Infow("Metrics collection disabled")
	}


	// Collect events if enabled
	if h.config.IncludeEvents && h.eventStore != nil && h.componentsRegistry != nil {
		since := time.Now().Add(-h.config.EventsLookback)
		log.Logger.Infow("healthexporter: Collecting events...", "since", since.Format("15:04:05 PDT"))
		
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
		log.Logger.Infow("healthexporter: Collected events from all components", "totalEvents", len(allEvents))
		
		// Show sample events for debugging
		if len(allEvents) > 0 {
			for i, event := range allEvents {
				if i >= 10 { // Show max 10 samples
					log.Logger.Infow("healthexporter: ... and more", "count", len(allEvents)-10)
					break
				}
				log.Logger.Infow("healthexporter: Event", "event", event)
			}
		}				
	} else {
		log.Logger.Infow("Events collection disabled in config")
	}
	
	// Collect component data (actual numbers/metrics) if enabled
	if h.config.IncludeComponentData && h.componentsRegistry != nil {
		log.Logger.Infow("healthexporter: Collecting component data...")
		componentData := h.collectComponentData()
		data.ComponentData = componentData
		log.Logger.Infow("healthexporter: Collected component data", "componentCount", len(componentData))
		
		// Show sample component data
		if len(componentData) > 0 {
			count := 0
			for componentName, componentResult := range componentData {
				if count >= 10 { // Show max 10 samples
					log.Logger.Infow("healthexporter: ... and more", "remainingCount", len(componentData)-10)
					break
				}
				log.Logger.Infow("healthexporter: Component data", "component", componentName, "data", componentResult)
				count++
			}
		}
	} else {
		log.Logger.Infow("Component data collection disabled")
	}
	
	log.Logger.Infow("healthexporter: Data collection completed!")
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
		log.Logger.Infow("healthexporter: Collecting health states...", "component", componentName, "health_states", healthStates)
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
	log.Logger.Infow("healthexporter: Converting to OTLP format...", "data", data)
	
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
							Name:    "gpud-health-exporter",
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
							Name:    "gpud-health-exporter",
							Version: "1.0.0",
						},
						LogRecords: h.convertToOTLPLogs(data),
					},
				},
			},
		},
	}

	return &OTLPData{
		Metrics: metricsData,    // machine_info, metrics
		Logs:    logsData,       // machine_info, events, component data
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
	log.Logger.Infow("healthexporter: Creating OTLP resource...")
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
	log.Logger.Infow("healthexporter: Creating OTLP metrics...")
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
	log.Logger.Infow("healthexporter: Created OTLP metrics...", "otlpMetrics", otlpMetrics)

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
	log.Logger.Infow("healthexporter: Creating OTLP logs...")
	var logRecords []*logsv1.LogRecord
	
	// Add events as log records
	if len(data.Events) > 0 {
		log.Logger.Infow("healthexporter: Converting events to OTLP logs", "count", len(data.Events))
		log.Logger.Infow("healthexporter: Events", "events", data.Events)
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
		log.Logger.Infow("healthexporter: Converting component data to OTLP logs", "count", len(data.ComponentData))
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

	log.Logger.Infow("healthexporter: Created OTLP log records", "total", len(logRecords))
	return logRecords
}

// sendHealthData sends the collected data to the global health endpoint using OTLP format
// TODO: limit the size of the data to be sent
// TODO: add a CLI option to send the data to a local file
func (h *healthExporter) sendHealthData(ctx context.Context, data *HealthData) error {

	// Convert to OTLP format
	otlpRequest := h.convertToOTLP(data)
	log.Logger.Infow("healthexporter: After converting to OTLP protobuf format...", "otlpRequest", otlpRequest)
	
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
		} else {
			log.Logger.Infow("healthexporter: Successfully sent metrics data", 
				"collection_id", data.CollectionID,
				"size_bytes", len(metricsBytes))
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
		} else {
			log.Logger.Infow("healthexporter: Successfully sent logs data", 
				"collection_id", data.CollectionID,
				"size_bytes", len(logsBytes))
		}
	}
	
	log.Logger.Infow("healthexporter: Successfully sent combined OTLP (metrics + logs)")
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
	
	log.Logger.Infow("healthexporter: OTLP request", "type", dataType, "size", len(reqData), "reqData", reqData)
	
	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", h.config.Endpoint, bytes.NewBuffer(reqData))
	if err != nil {
		log.Logger.Errorw("Failed to create HTTP request", "type", dataType, "error", err)
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "gpud-health-exporter")
	req.Header.Set("X-Machine-ID", h.config.MachineID)
	req.Header.Set("X-Data-Type", dataType) // Add data type header to help receiver
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
	
	log.Logger.Infow("healthexporter: Successfully sent data", "type", dataType, "status", resp.StatusCode, "size", len(reqData))
	
	return nil
} 
