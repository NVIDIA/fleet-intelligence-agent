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

// Command fleetint-otelcol is a custom OpenTelemetry Collector distribution
// for the Fleet Intelligence Agent. It uses a SAK (Service Account Key) to
// authenticate with the Fleet Intelligence backend via the enrollment endpoint
// (SAK→JWT exchange) and enrolls each unique agent seen in incoming OTLP data.
package main

import (
	"log"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.opentelemetry.io/collector/extension/auth"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/processor/memorylimiterprocessor"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"

	healthcheckextension "github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension"

	"github.com/NVIDIA/fleet-intelligence-agent/otelcol/auth/sakauth"
)

func main() {
	factories, err := components()
	if err != nil {
		log.Fatalf("failed to build component factories: %v", err)
	}

	info := component.BuildInfo{
		Command:     "fleetint-otelcol",
		Description: "Fleet Intelligence OTel Collector (SAK auth)",
		Version:     "0.0.1",
	}

	cmd := otelcol.NewCommand(otelcol.CollectorSettings{
		BuildInfo: info,
		Factories: factories,
	})

	if err := cmd.Execute(); err != nil {
		log.Fatalf("collector exited with error: %v", err)
	}
}

func components() (otelcol.Factories, error) {
	var errs []error

	extensions, err := otelcol.MakeExtensionFactoriesMap(
		// SAK→JWT auth (Fleet Intelligence custom)
		sakauth.NewFactory(),
		// Health check endpoint for liveness/readiness probes
		healthcheckextension.NewFactory(),
	)
	if err != nil {
		errs = append(errs, err)
	}

	receivers, err := otelcol.MakeReceiverFactoriesMap(
		otlpreceiver.NewFactory(),
	)
	if err != nil {
		errs = append(errs, err)
	}

	processors, err := otelcol.MakeProcessorFactoriesMap(
		batchprocessor.NewFactory(),
		memorylimiterprocessor.NewFactory(),
	)
	if err != nil {
		errs = append(errs, err)
	}

	exporters, err := otelcol.MakeExporterFactoriesMap(
		otlphttpexporter.NewFactory(),
	)
	if err != nil {
		errs = append(errs, err)
	}

	// Suppress unused import warning — auth is used by the sakauth extension.
	_ = auth.NewClient

	if len(errs) > 0 {
		return otelcol.Factories{}, errs[0]
	}

	return otelcol.Factories{
		Extensions: extensions,
		Receivers:  receivers,
		Processors: processors,
		Exporters:  exporters,
	}, nil
}
