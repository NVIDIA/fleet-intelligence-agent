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

package writer

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/leptonai/gpud/pkg/healthexporter/collector"
	"github.com/leptonai/gpud/pkg/healthexporter/converter"
	"github.com/leptonai/gpud/pkg/log"
	"google.golang.org/protobuf/proto"
)

const (
	// defaultRetryDelay is the default delay between retry attempts
	defaultRetryDelay = 5 * time.Second
)

// HTTPWriter defines the interface for sending health data via HTTP
type HTTPWriter interface {
	Send(ctx context.Context, data *collector.HealthData, endpoint string, maxRetries int) error
}

// httpWriter implements the HTTPWriter interface
type httpWriter struct {
	httpClient    *http.Client
	otlpConverter converter.OTLPConverter
}

// NewHTTPWriter creates a new HTTP writer
func NewHTTPWriter(httpClient *http.Client, otlpConverter converter.OTLPConverter) HTTPWriter {
	return &httpWriter{
		httpClient:    httpClient,
		otlpConverter: otlpConverter,
	}
}

// Send sends health data to the specified endpoint
func (w *httpWriter) Send(ctx context.Context, data *collector.HealthData, endpoint string, maxRetries int) error {
	// Convert to OTLP format
	otlpData := w.otlpConverter.Convert(data)

	// Send metrics first
	if otlpData.Metrics != nil && len(otlpData.Metrics.ResourceMetrics) > 0 {
		metricsBytes, err := proto.Marshal(otlpData.Metrics)
		if err != nil {
			return fmt.Errorf("failed to marshal OTLP metrics data: %w", err)
		}

		if err := w.sendOTLPRequestWithRetry(ctx, metricsBytes, "metrics", data.CollectionID, endpoint, maxRetries); err != nil {
			log.Logger.Errorw("Failed to send metrics data after all retries",
				"collection_id", data.CollectionID,
				"error", err,
				"size_bytes", len(metricsBytes))
			// Continue to send logs even if metrics fail
		}
	}

	// Send logs
	if otlpData.Logs != nil && len(otlpData.Logs.ResourceLogs) > 0 {
		logsBytes, err := proto.Marshal(otlpData.Logs)
		if err != nil {
			return fmt.Errorf("failed to marshal OTLP logs data: %w", err)
		}

		if err := w.sendOTLPRequestWithRetry(ctx, logsBytes, "logs", data.CollectionID, endpoint, maxRetries); err != nil {
			return fmt.Errorf("failed to send critical logs data (includes events): %w", err)
		}
	}

	log.Logger.Infow("Successfully sent health data to endpoint", "endpoint", endpoint)
	return nil
}

// sendOTLPRequestWithRetry sends the OTLP data with retry logic
func (w *httpWriter) sendOTLPRequestWithRetry(ctx context.Context, reqData []byte, dataType, collectionID, endpoint string, maxRetries int) error {
	if maxRetries <= 0 {
		maxRetries = 1 // At least one attempt
	}

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := w.sendOTLPRequest(ctx, reqData, dataType, collectionID, endpoint)
		if err == nil {
			if attempt > 1 {
				log.Logger.Infow("Request succeeded after retries",
					"data_type", dataType,
					"collection_id", collectionID,
					"attempt", attempt,
					"total_attempts", maxRetries)
			}
			return nil
		}

		lastErr = err

		// If this was the last attempt, don't wait
		if attempt >= maxRetries {
			break
		}

		delay := defaultRetryDelay
		log.Logger.Warnw("Request failed, retrying",
			"data_type", dataType,
			"collection_id", collectionID,
			"attempt", attempt,
			"total_attempts", maxRetries,
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

	log.Logger.Errorw("All retry attempts failed",
		"data_type", dataType,
		"collection_id", collectionID,
		"total_attempts", maxRetries,
		"final_error", lastErr)

	return fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

// sendOTLPRequest sends a single OTLP request
func (w *httpWriter) sendOTLPRequest(ctx context.Context, reqData []byte, dataType, collectionID, endpoint string) error {
	contentType := "application/x-protobuf"

	// Get machine ID for HTTP header
	machineID, err := collector.GetMachineID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get machine ID: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(reqData))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "gpuhealth-exporter")
	req.Header.Set("X-Machine-ID", machineID)
	req.Header.Set("X-Data-Type", dataType)
	req.Header.Set("X-Collection-ID", collectionID)

	// Send request
	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP request failed: %s (status %d)", resp.Status, resp.StatusCode)
	}

	return nil
}
