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

// Package converter handles conversion of health data to different formats
package converter

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/NVIDIA/gpuhealth/internal/exporter/collector"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
)

// OTLPData holds both metrics and logs for OTLP export
type OTLPData struct {
	Metrics *metricsv1.MetricsData
	Logs    *logsv1.LogsData
}

// OTLPConverter defines the interface for converting health data to OTLP format
type OTLPConverter interface {
	Convert(data *collector.HealthData) *OTLPData
}

// otlpConverter implements the OTLPConverter interface
type otlpConverter struct{}

// NewOTLPConverter creates a new OTLP converter
func NewOTLPConverter() OTLPConverter {
	return &otlpConverter{}
}

// Convert converts HealthData to OTLP metrics and logs format
func (c *otlpConverter) Convert(data *collector.HealthData) *OTLPData {
	// Create shared resource for both metrics and logs
	resource := c.createOTLPResource(data)

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
						Metrics: c.convertMetricsToOTLP(data),
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
						LogRecords: c.convertToOTLPLogs(data),
					},
				},
			},
		},
	}

	return &OTLPData{
		Metrics: metricsData,
		Logs:    logsData,
	}
}

// createOTLPResource creates OTLP resource with machine info and identification
func (c *otlpConverter) createOTLPResource(data *collector.HealthData) *resourcev1.Resource {
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

// convertMetricsToOTLP converts health metrics to OTLP metrics format
func (c *otlpConverter) convertMetricsToOTLP(data *collector.HealthData) []*metricsv1.Metric {
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
								TimeUnixNano: uint64(metric.UnixMilliseconds) * 1_000_000,
								Value: &metricsv1.NumberDataPoint_AsDouble{
									AsDouble: metric.Value,
								},
								Attributes: c.convertLabelsToOTLPAttributes(metric.Labels),
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
		Name:        "gpuhealth_agent_collection_summary",
		Description: "Summary of GPU health data collection including counts of metrics, events, and components",
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
func (c *otlpConverter) convertLabelsToOTLPAttributes(labels map[string]string) []*commonv1.KeyValue {
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
func (c *otlpConverter) convertToOTLPLogs(data *collector.HealthData) []*logsv1.LogRecord {
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

	// Add component data as log records
	if len(data.ComponentData) > 0 {
		for componentName, componentResult := range data.ComponentData {
			componentInfo, ok := componentResult.(map[string]interface{})
			if !ok {
				continue
			}

			health := componentInfo["health"]
			reason := componentInfo["reason"]
			timeVal := componentInfo["time"]
			extraInfo := componentInfo["extra_info"]

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

			// Add optional fields
			if timeVal != nil {
				attributes = append(attributes, &commonv1.KeyValue{
					Key: "time",
					Value: &commonv1.AnyValue{
						Value: &commonv1.AnyValue_StringValue{StringValue: fmt.Sprintf("%v", timeVal)},
					},
				})
			}

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

// convertStructToOTLPAttributes converts a struct to OTLP attributes using reflection
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
			// Handle slices by converting to JSON string
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
