package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/leptonai/gpud/pkg/systemd"
	"github.com/urfave/cli"

	"github.com/NVIDIA/gpuhealth/internal/cmdutil"
	"github.com/NVIDIA/gpuhealth/internal/config"
)

func statusCommand(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	serverURL := cliContext.String("server-url")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	log.Logger.Debugw("starting status command")

	rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer rootCancel()

	log.Logger.Debugw("getting state file")
	stateFile, err := config.DefaultStateFile()
	if err != nil {
		return fmt.Errorf("failed to get state file: %w", err)
	}
	log.Logger.Debugw("successfully got state file")

	// Check if we have read permission to the state file
	if _, err := os.Open(stateFile); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("insufficient permissions to read state file %s. Please run with sudo", stateFile)
		}
		// If it's not a permission error, continue - the sqlite.Open call below will handle other issues
	}

	log.Logger.Debugw("opening state file for reading")
	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRO.Close()
	log.Logger.Debugw("successfully opened state file for reading")

	var active bool
	if systemd.SystemctlExists() {
		active, err = systemd.IsActive("gpuhealthd.service")
		if err != nil {
			return err
		}
		if !active {
			fmt.Printf("%s gpuhealthd.service is not active\n", cmdutil.WarningSign)
		} else {
			fmt.Printf("%s gpuhealthd.service is active\n", cmdutil.CheckMark)
		}
	}
	if !active {
		// fallback to process list
		// in case it's not using systemd
		proc, err := process.FindProcessByName(rootCtx, "gpuhealth")
		if err != nil {
			return err
		}
		if proc == nil {
			fmt.Printf("%s gpuhealth process is not running\n", cmdutil.WarningSign)
			return nil
		}

		fmt.Printf("%s gpuhealth process is running (PID %d)\n", cmdutil.CheckMark, proc.PID())
	}
	fmt.Printf("%s successfully checked gpuhealth status\n", cmdutil.CheckMark)

	// Check server health
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(serverURL + "/healthz")
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	fmt.Printf("%s successfully checked gpuhealth health\n", cmdutil.CheckMark)
	return nil
}
