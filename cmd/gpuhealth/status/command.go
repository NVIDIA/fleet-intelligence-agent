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

package status

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli"

	clientv1 "github.com/leptonai/gpud/client/v1"
	cmdcommon "github.com/leptonai/gpud/cmd/common"
	"github.com/leptonai/gpud/pkg/gpuhealthconfig"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/leptonai/gpud/pkg/systemd"
)

func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	listenAddress := cliContext.String("listen-address")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	log.Logger.Debugw("starting status command")

	rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer rootCancel()

	log.Logger.Debugw("getting state file")
	stateFile, err := gpuhealthconfig.DefaultStateFile()
	if err != nil {
		return fmt.Errorf("failed to get state file: %w", err)
	}
	log.Logger.Debugw("successfully got state file")

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
			fmt.Printf("%s gpuhealthd.service is not active\n", cmdcommon.WarningSign)
		} else {
			fmt.Printf("%s gpuhealthd.service is active\n", cmdcommon.CheckMark)
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
			fmt.Printf("%s gpuhealth process is not running\n", cmdcommon.WarningSign)
			return nil
		}

		fmt.Printf("%s gpuhealth process is running (PID %d)\n", cmdcommon.CheckMark, proc.PID())
	}
	fmt.Printf("%s successfully checked gpuhealth status\n", cmdcommon.CheckMark)

	if err := clientv1.BlockUntilServerReady(
		rootCtx,
		listenAddress,
	); err != nil {
		return err
	}
	fmt.Printf("%s successfully checked gpuhealth health\n", cmdcommon.CheckMark)

	return nil
}
