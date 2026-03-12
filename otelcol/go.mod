// SPDX-FileCopyrightText: Copyright (c) 2025, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

module github.com/NVIDIA/fleet-intelligence-agent/otelcol

go 1.24

require (
	// OTel Collector core
	go.opentelemetry.io/collector/component v0.123.0
	go.opentelemetry.io/collector/extension v0.123.0
	go.opentelemetry.io/collector/extension/auth v0.123.0
	go.opentelemetry.io/collector/exporter/otlphttpexporter v0.123.0
	go.opentelemetry.io/collector/otelcol v0.123.0
	go.opentelemetry.io/collector/processor/batchprocessor v0.123.0
	go.opentelemetry.io/collector/processor/memorylimiterprocessor v0.123.0
	go.opentelemetry.io/collector/receiver/otlpreceiver v0.123.0

	// OTel Collector contrib
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension v0.123.0

	// gRPC credentials (required by auth.Client interface)
	google.golang.org/grpc v1.72.0
)
