package command

import (
	"fmt"

	"github.com/urfave/cli"

	cmdcompact "github.com/leptonai/gpud/cmd/gpuhealth/compact"
	cmdinject "github.com/leptonai/gpud/cmd/gpuhealth/inject"
	cmdmachineinfo "github.com/leptonai/gpud/cmd/gpuhealth/machine-info"
	cmdmetadata "github.com/leptonai/gpud/cmd/gpuhealth/metadata"
	cmdrun "github.com/leptonai/gpud/cmd/gpuhealth/run"
	cmdscan "github.com/leptonai/gpud/cmd/gpuhealth/scan"
	cmdstatus "github.com/leptonai/gpud/cmd/gpuhealth/status"
	"github.com/leptonai/gpud/pkg/gpuhealthconfig"
	"github.com/leptonai/gpud/version"
)

func App() *cli.App {
	app := cli.NewApp()

	app.Name = "gpuhealth"
	app.Usage = "NVIDIA GPU health monitoring and reporting tool"
	app.Version = version.Version
	app.Description = "Use this tool to monitor the health of your NVIDIA GPUs and export metrics for analysis"

	app.Commands = []cli.Command{
		{
			Name:    "scan",
			Aliases: []string{"check", "s"},
			Usage:   "quickly scans the host for any major issues",
			Action:  cmdscan.CreateCommand(),
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},

				&cli.IntFlag{
					Name:  "gpu-count",
					Usage: "specifies the expected GPU count",
					Value: 0,
				},
				&cli.StringFlag{
					Name:  "infiniband-expected-port-states",
					Usage: "set the infiniband expected port states in JSON (leave empty for default, useful for testing)",
				},
				&cli.StringFlag{
					Name:  "nfs-checker-configs",
					Usage: "set the NFS checker group configs in JSON (leave empty for default, useful for testing)",
				},
				cli.StringFlag{
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
			Action: cmdrun.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
				&cli.StringFlag{
					Name:  "log-file",
					Usage: "set the log file path (set empty to stdout/stderr)",
					Value: "",
				},
				&cli.StringFlag{
					Name:  "listen-address",
					Usage: "set the listen address",
					Value: fmt.Sprintf("0.0.0.0:%d", gpuhealthconfig.DefaultHealthPort),
				},
				&cli.BoolFlag{
					Name:  "pprof",
					Usage: "enable pprof (default: false)",
				},
				&cli.DurationFlag{
					Name:  "retention-period",
					Usage: "set the time period to retain metrics for (once elapsed, old records are compacted/purged)",
					Value: gpuhealthconfig.DefaultRetentionPeriod.Duration,
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
					Name:  "infiniband-expected-port-states",
					Usage: "set the infiniband expected port states in JSON (leave empty for default, useful for testing)",
				},
				&cli.StringFlag{
					Name:  "nfs-checker-configs",
					Usage: "set the NFS checker group configs in JSON (leave empty for default, useful for testing)",
				},

				cli.StringFlag{
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
			},
		},
		{
			Name:    "status",
			Aliases: []string{"st"},
			Usage:   "checks the status of the gpuhealth server",
			Action:  cmdstatus.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
				&cli.StringFlag{
					Name:  "listen-address",
					Usage: "set the listen address",
					Value: fmt.Sprintf("http://localhost:%d", gpuhealthconfig.DefaultHealthPort),
				},
			},
		},
		{
			Name:      "machine-info",
			Usage:     "gets machine information (useful for debugging)",
			UsageText: "gpuhealth machine-info",
			Action:    cmdmachineinfo.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
			},
		},
		{
			Name:   "metadata",
			Usage:  "inspects/updates the metadata table",
			Action: cmdmetadata.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
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
			Action: cmdcompact.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level,l",
					Usage: "set the logging level [debug, info, warn, error, fatal, panic, dpanic]",
				},
			},
		},
		{
			Name:   "inject",
			Usage:  "inject faults into components for testing",
			Action: cmdinject.Command,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "component,c",
					Usage:    "component name to inject fault into (required)",
					Required: true,
				},
				&cli.StringFlag{
					Name:  "fault-type",
					Usage: "fault type to inject into the component (component-error or event)",
				},
				&cli.StringFlag{
					Name:  "fault-message",
					Usage: "message to inject into the component",
				},
				&cli.StringFlag{
					Name:  "event-type",
					Usage: "type of the event to inject into the component",
				},
				&cli.StringFlag{
					Name:  "address",
					Usage: "gpuhealth server address",
				},
			},
		},
	}

	return app
}
