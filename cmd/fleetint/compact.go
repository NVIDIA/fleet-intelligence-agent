package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/netutil"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/leptonai/gpud/pkg/systemd"
	"github.com/urfave/cli"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/config"
)

func compactCommand(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	log.Logger.Debugw("starting compact command")

	if systemd.SystemctlExists() {
		active, err := systemd.IsActive("fleetintd.service")
		if err != nil {
			return err
		}
		if active {
			return fmt.Errorf("fleetintd service is running (must be stopped before running compact)")
		}
	}

	portOpen := netutil.IsPortOpen(config.DefaultHealthPort) // fleetint uses port 15133
	if portOpen {
		return fmt.Errorf("fleetint is running on port %d (must be stopped before running compact)", config.DefaultHealthPort)
	}

	log.Logger.Infow("successfully checked fleetintd is not running")

	rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer rootCancel()

	stateFile, err := config.DefaultStateFile()
	if err != nil {
		return fmt.Errorf("failed to get state file: %w", err)
	}

	// Check if we have write permission to the state file
	if _, err := os.OpenFile(stateFile, os.O_WRONLY, 0); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("insufficient permissions to write to state file %s. Please run with sudo", stateFile)
		}
		// If it's not a permission error, continue - the file might not exist yet or have other issues
		// that will be handled by the sqlite.Open call below
	}

	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRW.Close()

	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRO.Close()

	dbSize, err := sqlite.ReadDBSize(rootCtx, dbRO)
	if err != nil {
		return fmt.Errorf("failed to read state file size: %w", err)
	}
	log.Logger.Infow("state file size before compact", "size", humanize.Bytes(dbSize))

	if err := sqlite.Compact(rootCtx, dbRW); err != nil {
		return fmt.Errorf("failed to compact state file: %w", err)
	}

	dbSize, err = sqlite.ReadDBSize(rootCtx, dbRO)
	if err != nil {
		return fmt.Errorf("failed to read state file size: %w", err)
	}
	log.Logger.Infow("state file size after compact", "size", humanize.Bytes(dbSize))

	fmt.Printf("%s successfully compacted state file\n", cmdutil.CheckMark)
	return nil
}
