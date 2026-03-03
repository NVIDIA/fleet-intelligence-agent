package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/urfave/cli"
	"go.uber.org/zap"

	componentsnvidiagpucounts "github.com/leptonai/gpud/components/accelerator/nvidia/gpu-counts"
	nvidiainfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	infinibandtypes "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	"github.com/leptonai/gpud/pkg/log"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/scan"
)

func scanCreateCommand() func(*cli.Context) error {
	return func(cliContext *cli.Context) error {
		return cmdScan(
			cliContext.String("log-level"),
			cliContext.Int("gpu-count"),
			cliContext.String("infiniband-expected-port-states"),
			cliContext.String("infiniband-class-root-dir"),
		)
	}
}

func cmdScan(logLevel string, gpuCount int, infinibandExpectedPortStates string, ibClassRootDir string) error {
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	log.Logger.Debugw("starting scan command")

	if gpuCount > 0 {
		componentsnvidiagpucounts.SetDefaultExpectedGPUCounts(componentsnvidiagpucounts.ExpectedGPUCounts{
			Count: gpuCount,
		})

		log.Logger.Infow("set gpu count", "gpuCount", gpuCount)
	}

	if len(infinibandExpectedPortStates) > 0 {
		var expectedPortStates infinibandtypes.ExpectedPortStates
		if err := json.Unmarshal([]byte(infinibandExpectedPortStates), &expectedPortStates); err != nil {
			return err
		}
		nvidiainfiniband.SetDefaultExpectedPortStates(expectedPortStates)

		log.Logger.Infow("set infiniband expected port states", "infinibandExpectedPortStates", infinibandExpectedPortStates)
	}

	opts := []scan.Option{
		scan.WithInfinibandClassRootDir(ibClassRootDir),
	}
	if zapLvl.Level() <= zap.DebugLevel { // e.g., info, warn, error
		opts = append(opts, scan.WithDebug(true))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err = scan.Scan(ctx, opts...); err != nil {
		return err
	}

	return nil
}
