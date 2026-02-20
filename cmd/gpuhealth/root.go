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

package main

import (
	"fmt"

	"github.com/urfave/cli"

	"github.com/NVIDIA/gpuhealth/internal/config"
	"github.com/NVIDIA/gpuhealth/internal/version"
)

func App() *cli.App {
	app := cli.NewApp()

	app.Name = "gpuhealth"
	app.Usage = "NVIDIA GPU health monitoring and reporting tool"
	app.Version = version.Version
	app.Description = "Use this tool to monitor the health of your NVIDIA GPUs and export metrics for analysis"

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("%s v%s (go %s, built %s)\n",
			c.App.Name,
			version.Version,
			version.GoVersion,
			version.BuildTimestamp,
		)
	}

	app.Commands = []cli.Command{
		{
			Name:    "scan",
			Aliases: []string{"check", "s"},
			Usage:   "quickly scans the host for any major issues",
			Action:  scanCreateCommand(),
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error]",
				},
				&cli.IntFlag{
					Name:  "gpu-count",
					Usage: "specifies the expected GPU count",
					Value: 0,
				},
				&cli.StringFlag{
					Name:   "infiniband-expected-port-states",
					Usage:  "set the infiniband expected port states in JSON (leave empty for default, useful for testing)",
					Hidden: true, // only for testing - auto-detected by default
				},
				&cli.StringFlag{
					Name:   "infiniband-class-root-dir",
					Usage:  "sets the infiniband class root directory (leave empty for default)",
					Value:  "",
					Hidden: true, // only for testing
				},
			},
		},
		{
			Name:   "run",
			Usage:  "starts the gpuhealth server",
			Action: runCommand,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error]",
				},
				&cli.StringFlag{
					Name:  "log-file",
					Usage: "set the log file path (set empty to stdout/stderr)",
					Value: "",
				},
				&cli.StringFlag{
					Name:  "listen-address",
					Usage: "set the listen address",
					Value: config.DefaultListenAddress,
				},
				&cli.DurationFlag{
					Name:  "retention-period",
					Usage: "set the time period to retain metrics for (once elapsed, old records are automatically purged)",
					Value: config.DefaultRetentionPeriod.Duration,
				},
				cli.StringFlag{
					Name:  "components",
					Usage: "sets the components to enable (comma-separated, leave empty for default to enable all components, set 'none' or any other non-matching value to disable all components, prefix component name with '-' to disable it)",
					Value: "",
				},
				&cli.IntFlag{
					Name:  "gpu-count",
					Usage: "specifies the expected GPU count",
					Value: 0,
				},
				&cli.StringFlag{
					Name:   "infiniband-expected-port-states",
					Usage:  "set the infiniband expected port states in JSON (leave empty for default, useful for testing)",
					Hidden: true, // only for testing - auto-detected by default
				},
				&cli.StringFlag{
					Name:   "infiniband-class-root-dir",
					Usage:  "sets the infiniband class root directory (leave empty for default)",
					Value:  "",
					Hidden: true, // only for testing
				},
				&cli.BoolFlag{
					Name:  "offline-mode",
					Usage: "enable offline mode to write telemetry data and machine information to a file",
				},
				&cli.StringFlag{
					Name:  "path",
					Usage: "path where file will be written (required when --offline-mode is used)",
				},
				&cli.StringFlag{
					Name:  "duration",
					Usage: "duration for offline mode run in HH:MM:SS format (required when --offline-mode is used)",
				},
				&cli.StringFlag{
					Name:  "format",
					Usage: "output format for offline mode [json|csv]",
					Value: "json",
				},
				&cli.BoolFlag{
					Name:  "enable-dcgm-policy",
					Usage: "enable DCGM policy violation monitoring for all policies (XID, PCIe, DBE, NVLink, Power, Thermal, Page Retirement) (default: false)",
				},
				&cli.BoolFlag{
					Name:  "enable-fault-injection",
					Usage: "enable fault injection endpoint for testing (only accessible from localhost, default: false)",
				},
			},
		},
		{
			Name:    "status",
			Aliases: []string{"st"},
			Usage:   "checks the status of the gpuhealth server",
			Action:  statusCommand,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error]",
				},
				&cli.StringFlag{
					Name:  "server-url",
					Usage: "set the server URL to connect to",
					Value: config.DefaultClientURL,
				},
			},
		},
		{
			Name:      "machine-info",
			Usage:     "gets machine information (useful for debugging)",
			UsageText: "gpuhealth machine-info",
			Action:    machineInfoCommand,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error]",
				},
			},
		},
		{
			Name:   "metadata",
			Usage:  "inspects/updates the metadata table",
			Action: metadataCommand,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error]",
				},
				&cli.StringFlag{
					Name:  "set-key",
					Usage: "metadata key to set/update",
				},
				&cli.StringFlag{
					Name:  "set-value",
					Usage: "value to set for the metadata key",
				},
			},
		},
		{
			Name:   "compact",
			Usage:  "compacts the gpuhealth state database to reduce disk usage (gpuhealth daemon/server must be stopped)",
			Action: compactCommand,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error]",
				},
			},
		},
		{
			Name:   "enroll",
			Usage:  "enroll the agent with GPU Health backend endpoints and credentials",
			Action: enrollCommand,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "endpoint",
					Usage:    "base endpoint URL (required)",
					Required: true,
				},
				&cli.StringFlag{
					Name:     "token",
					Usage:    "authentication token (required)",
					Required: true,
				},
			},
		},
		{
			Name:   "unenroll",
			Usage:  "un-enroll the agent from GPU Health backend (removes credentials and endpoints)",
			Action: unenrollCommand,
		},
		{
			Name:   "inject",
			Usage:  "inject faults into components for testing",
			Action: injectCommand,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "server-url",
					Usage: "set the server URL to connect to",
					Value: config.DefaultClientURL,
				},
				&cli.StringFlag{
					Name:     "component,c",
					Usage:    "component name to inject fault into (required)",
					Required: true,
				},
				&cli.StringFlag{
					Name:  "fault-type",
					Usage: "fault type to inject into the component (component-error or event or kernel-message)",
				},
				&cli.StringFlag{
					Name:  "fault-message",
					Usage: "message to inject into the component",
				},
				&cli.StringFlag{
					Name:  "event-type",
					Usage: "type of the event to inject into the component",
				},
				&cli.BoolFlag{
					Name:  "clear",
					Usage: "clear injected faults from the component instead of injecting new ones",
				},
			},
		},
	}

	return app
}
