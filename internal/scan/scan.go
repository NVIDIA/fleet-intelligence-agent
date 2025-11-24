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

// Package scan provides system scanning functionality for GPU health monitoring.
// This implementation is based on the upstream gpud scan package but maintained
// independently to give gpuhealth full control over the scanning behavior.
package scan

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	cmdcommon "github.com/leptonai/gpud/cmd/common"
	"github.com/leptonai/gpud/components"
	componentsacceleratornvidiainfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	nvidiacommon "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/log"
	nvidiadcgm "github.com/leptonai/gpud/pkg/nvidia-query/dcgm"
	nvidiainfiniband "github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	infinibandclass "github.com/leptonai/gpud/pkg/nvidia-query/infiniband/class"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"

	"github.com/NVIDIA/gpuhealth/internal/machineinfo"
	"github.com/NVIDIA/gpuhealth/internal/registry"
)

// Op holds the configuration for a scan operation.
type Op struct {
	infinibandClassRootDir string
	debug                  bool
	failureInjector        *components.FailureInjector
}

// Option represents a functional option for configuring the scan operation.
type Option func(*Op)

func (op *Op) applyOpts(opts []Option) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.infinibandClassRootDir == "" {
		op.infinibandClassRootDir = infinibandclass.DefaultRootDir
	}

	return nil
}

// WithInfinibandClassRootDir specifies the root directory of the InfiniBand class.
func WithInfinibandClassRootDir(p string) Option {
	return func(op *Op) {
		op.infinibandClassRootDir = p
	}
}

// WithFailureInjector sets the failure injector for testing purposes.
func WithFailureInjector(injector *components.FailureInjector) Option {
	return func(op *Op) {
		op.failureInjector = injector
	}
}

// WithDebug enables debug mode for the scan operation.
func WithDebug(b bool) Option {
	return func(op *Op) {
		op.debug = b
	}
}

func printSummary(result components.CheckResult) {
	header := cmdcommon.CheckMark
	if result.HealthStateType() != apiv1.HealthStateTypeHealthy {
		header = cmdcommon.WarningSign
	}
	fmt.Printf("%s %s\n", header, result.Summary())
	fmt.Println(result.String())
	println()
}

// Scan performs a comprehensive system scan to detect any major issues with
// GPU health, infiniband connectivity, NFS mounts, and other critical components.
// It returns an error if the scan fails to execute, but prints warnings for
// detected issues.
func Scan(ctx context.Context, opts ...Option) error {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	fmt.Printf("\n\n%s scanning the host (GOOS %s)\n\n", cmdcommon.InProgress, runtime.GOOS)

	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		return err
	}

	mi, err := machineinfo.GetMachineInfo(nvmlInstance)
	if err != nil {
		return err
	}
	fmt.Printf("\n%s machine info\n", cmdcommon.CheckMark)
	mi.RenderTable(os.Stdout)

	if mi.GPUInfo != nil && mi.GPUInfo.Product != "" {
		threshold, err := nvidiainfiniband.SupportsInfinibandPortRate(mi.GPUInfo.Product)
		if err == nil {
			log.Logger.Infow("setting default expected port states", "product", mi.GPUInfo.Product, "at_least_ports", threshold.AtLeastPorts, "at_least_rate", threshold.AtLeastRate)
			componentsacceleratornvidiainfiniband.SetDefaultExpectedPortStates(threshold)
		}
	}

	// Initialize DCGM instance for DCGM-based health checks
	dcgmInstance, err := nvidiadcgm.New()
	if err != nil {
		return err
	}

	// For scan mode, create a health cache
	dcgmHealthCache := nvidiadcgm.NewHealthCache(ctx, dcgmInstance, time.Minute)

	// Create field value cache for GPU device fields (placeholder)
	// Field watching will be set up after components register their fields
	// Note: CPU component manages its own field watching separately
	dcgmFieldValueCache := nvidiadcgm.NewFieldValueCache(ctx, dcgmInstance, time.Minute)

	defer func() {
		if dcgmHealthCache != nil {
			dcgmHealthCache.Stop()
		}
		if dcgmFieldValueCache != nil {
			dcgmFieldValueCache.Stop()
		}
		if err := dcgmInstance.Shutdown(); err != nil {
			log.Logger.Warnw("DCGM shutdown failed", "error", err)
		}
	}()

	gpudInstance := &components.GPUdInstance{
		RootCtx: ctx,

		MachineID: mi.MachineID,

		NVMLInstance:        nvmlInstance,
		DCGMInstance:        dcgmInstance,
		DCGMHealthCache:     dcgmHealthCache,
		DCGMFieldValueCache: dcgmFieldValueCache,
		NVIDIAToolOverwrites: nvidiacommon.ToolOverwrites{
			InfinibandClassRootDir: op.infinibandClassRootDir,
		},

		EventStore:       nil,
		RebootEventStore: nil,

		MountPoints:  []string{"/"},
		MountTargets: []string{"/var/lib/kubelet"},

		FailureInjector: op.failureInjector,
	}

	// Initialize all components first using gpuhealth's component registry
	var initializedComponents []components.Component
	for _, c := range registry.All() {
		comp, err := c.InitFunc(gpudInstance)
		if err != nil {
			return err
		}
		if !comp.IsSupported() {
			continue
		}
		initializedComponents = append(initializedComponents, comp)
	}

	// Perform one health check to populate the cache
	if err := dcgmHealthCache.Poll(); err != nil {
		log.Logger.Warnw("DCGM health check failed", "error", err)
	}

	// Set up DCGM field watching after all components have registered their fields
	if err := dcgmFieldValueCache.SetupFieldWatching(); err != nil {
		log.Logger.Warnw("failed to set up DCGM field watching", "error", err)
	}

	// Perform one field value poll to populate the cache
	if err := dcgmFieldValueCache.Poll(); err != nil {
		log.Logger.Warnw("DCGM field value poll failed", "error", err)
	}

	// Run checks on all initialized components
	for _, c := range initializedComponents {
		printSummary(c.Check())
	}

	fmt.Printf("\n\n%s scan complete\n\n", cmdcommon.CheckMark)
	return nil
}
