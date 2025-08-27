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

package healthexporter

/*
This file is used to mock the global health endpoint for testing.
It is used to test the health exporter and the health server.
TODO: Remove this file once we have a real endpoint.
*/

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/healthexporter/collector"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
)

// MockEndpoint represents a mock global health endpoint for testing
type MockEndpoint struct {
	server      *http.Server
	port        int
	mu          sync.RWMutex
	requests    []*ReceivedHealthData
	isRunning   bool
	pendingData map[string]*collector.HealthData // Track partial data by machine ID
}

// ReceivedHealthData represents health data received by the mock endpoint
type ReceivedHealthData struct {
	Data      *collector.HealthData `json:"data"`
	Timestamp time.Time             `json:"timestamp"`
	Headers   http.Header           `json:"headers"`
}

// NewMockEndpoint creates a new mock global health endpoint
func NewMockEndpoint(port int) *MockEndpoint {
	return &MockEndpoint{
		port:        port,
		requests:    make([]*ReceivedHealthData, 0),
		pendingData: make(map[string]*collector.HealthData),
	}
}

// Start starts the mock endpoint server
func (m *MockEndpoint) Start() error {
	mux := http.NewServeMux()

	// Health bulk endpoint
	mux.HandleFunc("/api/v1/health/bulk", m.handleHealthBulk)

	// Status endpoint for checking if mock is running
	mux.HandleFunc("/status", m.handleStatus)

	// Clear endpoint for testing
	mux.HandleFunc("/clear", m.handleClear)

	// Get requests endpoint for testing
	mux.HandleFunc("/requests", m.handleGetRequests)

	m.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", m.port),
		Handler: mux,
	}

	go func() {
		log.Logger.Infow("mock health endpoint: starting server", "port", m.port)
		if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Logger.Errorw("mock health endpoint: server error", "error", err)
		}
	}()

	m.mu.Lock()
	m.isRunning = true
	m.mu.Unlock()

	// Wait a bit for server to start
	time.Sleep(100 * time.Millisecond)

	return nil
}

// Stop stops the mock endpoint server
func (m *MockEndpoint) Stop() error {
	m.mu.Lock()
	m.isRunning = false
	m.mu.Unlock()

	if m.server != nil {
		log.Logger.Info("mock health endpoint: stopping server")
		return m.server.Close()
	}
	return nil
}

// GetRequests returns all received health data requests
func (m *MockEndpoint) GetRequests() []*ReceivedHealthData {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to avoid race conditions
	requests := make([]*ReceivedHealthData, len(m.requests))
	copy(requests, m.requests)
	return requests
}

// ClearRequests clears all received requests
func (m *MockEndpoint) ClearRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = make([]*ReceivedHealthData, 0)
}

// IsRunning returns whether the mock server is running
func (m *MockEndpoint) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isRunning
}

// URL returns the base URL of the mock endpoint
func (m *MockEndpoint) URL() string {
	return fmt.Sprintf("http://localhost:%d", m.port)
}

// HealthBulkURL returns the full URL for the health bulk endpoint
func (m *MockEndpoint) HealthBulkURL() string {
	return fmt.Sprintf("%s/api/v1/health/bulk", m.URL())
}

// handleHealthBulk handles POST requests to the health bulk endpoint
func (m *MockEndpoint) handleHealthBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Logger.Errorw("mock health endpoint: failed to read request body", "error", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Parse OTLP protobuf health data
	var healthData collector.HealthData
	contentType := r.Header.Get("Content-Type")
	dataType := r.Header.Get("X-Data-Type")         // Get data type (metrics or logs)
	collectionID := r.Header.Get("X-Collection-ID") // Get collection ID to correlate logs and metrics

	if contentType != "application/x-protobuf" {
		log.Logger.Errorw("mock health endpoint: only OTLP protobuf format supported", "content_type", contentType)
		http.Error(w, "Only OTLP protobuf format supported (Content-Type: application/x-protobuf)", http.StatusUnsupportedMediaType)
		return
	}

	log.Logger.Infow("mock health endpoint: received OTLP data", "data_type", dataType, "size", len(body))

	// Handle OTLP protobuf format
	if err := m.parseOTLPData(body, &healthData, dataType); err != nil {
		log.Logger.Errorw("mock health endpoint: failed to parse OTLP data", "error", err, "data_type", dataType)
		http.Error(w, "Failed to parse OTLP data", http.StatusBadRequest)
		return
	}

	// Set collection ID from header
	if collectionID != "" {
		healthData.CollectionID = collectionID
	}

	// Merge with pending data if machine ID exists
	m.mu.Lock()
	machineID := healthData.MachineID
	if machineID == "" {
		machineID = "unknown"
	}

	// Get or create pending data for this machine
	if existing, exists := m.pendingData[machineID]; exists {
		// Merge new data with existing
		mergedData := m.mergeHealthData(existing, &healthData, dataType)
		healthData = *mergedData
	}

	// Update pending data
	m.pendingData[machineID] = &healthData

	// Store the complete request (always store, even if partial)
	receivedData := &ReceivedHealthData{
		Data:      &healthData,
		Timestamp: time.Now().UTC(),
		Headers:   r.Header,
	}

	m.requests = append(m.requests, receivedData)
	m.mu.Unlock()

	log.Logger.Infow("mock health endpoint: received health data",
		"collection_id", healthData.CollectionID,
		"machine_id", healthData.MachineID,
		"timestamp", healthData.Timestamp,
		"metrics_count", len(healthData.Metrics),
		"events_count", len(healthData.Events),
		"component_data_count", len(healthData.ComponentData),
		"data_type", dataType)

	// Return simple success response
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)

	response := fmt.Sprintf("SUCCESS: Health data (%s) received successfully for machine %s at %s",
		dataType, healthData.MachineID, time.Now().UTC().Format(time.RFC3339))

	if _, err := w.Write([]byte(response)); err != nil {
		log.Logger.Errorw("mock health endpoint: failed to write response", "error", err)
	}
}

// handleStatus handles GET requests to the status endpoint
func (m *MockEndpoint) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	m.mu.RLock()
	requestCount := len(m.requests)
	isRunning := m.isRunning
	m.mu.RUnlock()

	response := map[string]interface{}{
		"status":        "ok",
		"running":       isRunning,
		"port":          m.port,
		"request_count": requestCount,
		"timestamp":     time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleClear handles POST requests to clear stored requests
func (m *MockEndpoint) handleClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	m.ClearRequests()

	response := map[string]interface{}{
		"status":  "success",
		"message": "Requests cleared",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGetRequests handles GET requests to retrieve stored requests
func (m *MockEndpoint) handleGetRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	requests := m.GetRequests()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(requests)
}

// parseOTLPAttributesToStruct parses OTLP attributes into a struct using reflection
// It handles nested fields with dot notation (e.g., "cpuInfo.type", "gpuInfo.product")
func parseOTLPAttributesToStruct(attributes []*commonv1.KeyValue, target interface{}) {
	if target == nil {
		return
	}

	val := reflect.ValueOf(target)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return
	}

	// Parse attributes and set struct fields
	for _, attr := range attributes {
		setNestedField(val, attr.Key, attr.Value.GetStringValue())
	}
}

// setNestedField sets a field value in a struct, handling nested fields with dot notation
func setNestedField(val reflect.Value, fieldPath string, stringValue string) {
	if stringValue == "" {
		return
	}

	// Split the field path by dots
	parts := strings.Split(fieldPath, ".")
	currentVal := val

	// Navigate through nested structs
	for i, part := range parts {
		if currentVal.Kind() != reflect.Struct {
			return
		}

		// Find the field in the current struct
		fieldIndex := -1
		currentType := currentVal.Type()

		for j := 0; j < currentVal.NumField(); j++ {
			fieldType := currentType.Field(j)

			// Skip unexported fields
			if !fieldType.IsExported() {
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

			if fieldName == part {
				fieldIndex = j
				break
			}
		}

		if fieldIndex == -1 {
			return // Field not found
		}

		field := currentVal.Field(fieldIndex)
		fieldType := currentType.Field(fieldIndex)

		// If this is the last part, set the value
		if i == len(parts)-1 {
			if !field.CanSet() {
				return
			}

			// Set field value based on its type
			switch field.Kind() {
			case reflect.String:
				field.SetString(stringValue)
			case reflect.Int64:
				if intVal, err := strconv.ParseInt(stringValue, 10, 64); err == nil {
					field.SetInt(intVal)
				}
			case reflect.Uint64:
				if uintVal, err := strconv.ParseUint(stringValue, 10, 64); err == nil {
					field.SetUint(uintVal)
				}
			case reflect.Struct:
				// Handle time.Time specially
				if fieldType.Type.String() == "time.Time" {
					if timeVal, err := time.Parse(time.RFC3339, stringValue); err == nil {
						field.Set(reflect.ValueOf(timeVal))
					}
				}
			case reflect.Slice:
				// Handle slices by parsing JSON string back to slice
				if strings.HasPrefix(stringValue, "[") && strings.HasSuffix(stringValue, "]") {
					// Create a new slice of the appropriate type
					sliceType := field.Type()
					newSlice := reflect.New(sliceType).Interface()

					// Unmarshal JSON into the slice
					if err := json.Unmarshal([]byte(stringValue), newSlice); err == nil {
						field.Set(reflect.ValueOf(newSlice).Elem())
					}
				}
			}
			return
		}

		// Navigate to the next level
		if field.Kind() == reflect.Ptr {
			// If it's a pointer and nil, create a new instance
			if field.IsNil() {
				if !field.CanSet() {
					return
				}
				newVal := reflect.New(field.Type().Elem())
				field.Set(newVal)
			}
			currentVal = field.Elem()
		} else if field.Kind() == reflect.Struct {
			currentVal = field
		} else {
			return // Can't navigate further
		}
	}
}

// parses OTLP protobuf data and converts it to collector.HealthData format
func (m *MockEndpoint) parseOTLPData(data []byte, healthData *collector.HealthData, dataType string) error {
	// Parse based on expected data type first
	if dataType == "logs" {
		// Try parsing as OTLP logs first (events + component data)
		var logsData logsv1.LogsData
		log.Logger.Infow("healthexporter: Health Exporter: Parsing OTLP logs data", "dataType", dataType)
		if err := proto.Unmarshal(data, &logsData); err == nil && len(logsData.ResourceLogs) > 0 {
			return m.parseOTLPLogs(&logsData, healthData, dataType)
		}
	} else if dataType == "metrics" {
		// Try parsing as OTLP metrics first
		var metricsData metricsv1.MetricsData
		log.Logger.Infow("healthexporter: Health Exporter: Parsing OTLP metrics data", "dataType", dataType)
		if err := proto.Unmarshal(data, &metricsData); err == nil {
			return m.parseOTLPMetrics(&metricsData, healthData, dataType)
		}
	}

	// Fallback: try both formats if dataType doesn't match or is unknown
	var logsData logsv1.LogsData
	if err := proto.Unmarshal(data, &logsData); err == nil && len(logsData.ResourceLogs) > 0 {
		return m.parseOTLPLogs(&logsData, healthData, dataType)
	}

	var metricsData metricsv1.MetricsData
	if err := proto.Unmarshal(data, &metricsData); err == nil {
		return m.parseOTLPMetrics(&metricsData, healthData, dataType)
	}

	return fmt.Errorf("failed to parse as either OTLP logs or metrics")
}

// parseOTLPLogs parses OTLP logs data containing events and component data
func (m *MockEndpoint) parseOTLPLogs(logsData *logsv1.LogsData, healthData *collector.HealthData, dataType string) error {
	healthData.Timestamp = time.Now() // Use current time as fallback
	healthData.Events = []eventstore.Event{}
	healthData.ComponentData = make(map[string]interface{})

	if len(logsData.ResourceLogs) > 0 {
		rl := logsData.ResourceLogs[0]

		// Extract machine info from resource attributes using reflection
		if rl.Resource != nil {
			healthData.MachineInfo = &apiv1.MachineInfo{}

			// Parse machine.id separately since it's not part of MachineInfo struct
			for _, attr := range rl.Resource.Attributes {
				if attr.Key == "machine.id" {
					if stringVal := attr.Value.GetStringValue(); stringVal != "" {
						healthData.MachineID = stringVal
					}
					break
				}
			}

			// Parse all other attributes into MachineInfo using reflection
			parseOTLPAttributesToStruct(rl.Resource.Attributes, healthData.MachineInfo)
		}

		// Parse log records (events and component data)
		if len(rl.ScopeLogs) > 0 {
			for _, sl := range rl.ScopeLogs {
				for _, logRecord := range sl.LogRecords {
					// Determine log type from attributes
					logType := ""
					component := ""

					for _, attr := range logRecord.Attributes {
						if attr.Key == "log_type" {
							logType = attr.Value.GetStringValue()
						}
						if attr.Key == "component" {
							component = attr.Value.GetStringValue()
						}
					}

					// Convert based on log type
					if logType == "event" {
						// Parse as event
						event := eventstore.Event{
							Component: component,
							Time:      time.Unix(0, int64(logRecord.TimeUnixNano)),
							Message:   logRecord.Body.GetStringValue(),
						}

						// Extract additional event details from attributes
						for _, attr := range logRecord.Attributes {
							switch attr.Key {
							case "event_name":
								event.Name = attr.Value.GetStringValue()
							case "event_type":
								event.Type = attr.Value.GetStringValue()
							}
						}

						healthData.Events = append(healthData.Events, event)

					} else if logType == "component_data" {
						// Parse as component data with health information, time, and extra info
						componentInfo := map[string]interface{}{
							"component_name": component,
							"health":         "Unknown",
							"reason":         "No data",
						}

						// Extract all available fields from attributes
						for _, attr := range logRecord.Attributes {
							switch attr.Key {
							case "health":
								componentInfo["health"] = attr.Value.GetStringValue()
							case "reason":
								componentInfo["reason"] = attr.Value.GetStringValue()
							case "time":
								componentInfo["time"] = attr.Value.GetStringValue()
							case "extra_info":
								componentInfo["extra_info"] = attr.Value.GetStringValue()
							}
						}

						healthData.ComponentData[component] = componentInfo
					}
				}
			}
		}
	}

	return nil
}

// parseOTLPMetrics parses OTLP metrics data (fallback)
func (m *MockEndpoint) parseOTLPMetrics(metricsData *metricsv1.MetricsData, healthData *collector.HealthData, dataType string) error {
	// Convert OTLP back to collector.HealthData format (simplified for demo)
	healthData.Timestamp = time.Now() // Use current time as fallback

	if len(metricsData.ResourceMetrics) > 0 {
		rm := metricsData.ResourceMetrics[0]

		// Extract machine info from resource attributes using reflection
		if rm.Resource != nil {
			healthData.MachineInfo = &apiv1.MachineInfo{}

			// Parse machine.id separately since it's not part of MachineInfo struct
			for _, attr := range rm.Resource.Attributes {
				if attr.Key == "machine.id" {
					if stringVal := attr.Value.GetStringValue(); stringVal != "" {
						healthData.MachineID = stringVal
					}
					break
				}
			}

			// Parse all other attributes into MachineInfo using reflection
			parseOTLPAttributesToStruct(rm.Resource.Attributes, healthData.MachineInfo)
		}

		// Convert metrics
		if len(rm.ScopeMetrics) > 0 {
			for _, sm := range rm.ScopeMetrics {
				for _, metric := range sm.Metrics {
					if gauge := metric.GetGauge(); gauge != nil {
						for _, dp := range gauge.DataPoints {
							// Convert OTLP metric back to our format
							healthMetric := pkgmetrics.Metric{
								Name:             metric.Name,
								Value:            dp.GetAsDouble(),
								UnixMilliseconds: int64(dp.TimeUnixNano) / 1000000, // Convert nano to millis
							}

							// Extract component from description field
							// Description format: "Metric from component {component_name}"
							if metric.Description != "" {
								const prefix = "Metric from component "
								if len(metric.Description) > len(prefix) && metric.Description[:len(prefix)] == prefix {
									healthMetric.Component = metric.Description[len(prefix):]
								}
							}

							// Extract labels from OTLP attributes
							if len(dp.Attributes) > 0 {
								healthMetric.Labels = make(map[string]string)
								for _, attr := range dp.Attributes {
									if stringVal := attr.Value.GetStringValue(); stringVal != "" {
										healthMetric.Labels[attr.Key] = stringVal
									}
								}
							}

							healthData.Metrics = append(healthData.Metrics, healthMetric)
						}
					}
				}
			}
		}
	}

	return nil
}

// mergeHealthData merges new health data with existing pending data
func (m *MockEndpoint) mergeHealthData(existing, new *collector.HealthData, dataType string) *collector.HealthData {
	// Start with existing data
	merged := *existing

	// Update timestamp to latest
	if new.Timestamp.After(existing.Timestamp) {
		merged.Timestamp = new.Timestamp
	}

	// Merge based on data type
	switch dataType {
	case "metrics":
		// Update metrics and machine info from metrics request
		if len(new.Metrics) > 0 {
			merged.Metrics = new.Metrics
		}
		if new.MachineInfo != nil {
			merged.MachineInfo = new.MachineInfo
		}
		if new.MachineID != "" {
			merged.MachineID = new.MachineID
		}
	case "logs":
		// Update events and component data from logs request
		if len(new.Events) > 0 {
			merged.Events = new.Events
		}
		if len(new.ComponentData) > 0 {
			merged.ComponentData = new.ComponentData
		}
		// Also update machine info if not set
		if merged.MachineInfo == nil && new.MachineInfo != nil {
			merged.MachineInfo = new.MachineInfo
		}
		if merged.MachineID == "" && new.MachineID != "" {
			merged.MachineID = new.MachineID
		}
	}

	return &merged
}
