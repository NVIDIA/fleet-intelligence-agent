// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package scan

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	cmdcommon "github.com/NVIDIA/fleet-intelligence-sdk/cmd/common"
	"github.com/NVIDIA/fleet-intelligence-sdk/components"
	nvidiainfiniband "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/infiniband"
	"github.com/NVIDIA/fleet-intelligence-sdk/components/all"
	nvidiacommon "github.com/NVIDIA/fleet-intelligence-sdk/pkg/config/common"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	pkgmachineinfo "github.com/NVIDIA/fleet-intelligence-sdk/pkg/machine-info"
	nvidiadcgm "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/dcgm"
	nvidianvml "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/nvml"
)

func printSummary(result components.CheckResult) {
	header := cmdcommon.CheckMark
	if result.HealthStateType() != apiv1.HealthStateTypeHealthy {
		header = cmdcommon.WarningSign
	}
	fmt.Printf("%s %s\n", header, result.Summary())
	fmt.Println(result.String())
	println()
}

// Runs the scan operations.
func Scan(ctx context.Context, opts ...OpOption) error {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	fmt.Printf("\n\n%s scanning the host (GOOS %s)\n\n", cmdcommon.InProgress, runtime.GOOS)

	var nvmlInstance nvidianvml.Instance
	var err error
	if op.failureInjector != nil && (len(op.failureInjector.GPUUUIDsWithGPULost) > 0 ||
		len(op.failureInjector.GPUUUIDsWithGPURequiresReset) > 0 ||
		len(op.failureInjector.GPUUUIDsWithFabricStateHealthSummaryUnhealthy) > 0 ||
		op.failureInjector.GPUProductNameOverride != "") {
		// If failure injector is configured for NVML-level errors or product name override, use it
		nvmlInstance, err = nvidianvml.NewWithFailureInjector(&nvidianvml.FailureInjectorConfig{
			GPUUUIDsWithGPULost:                           op.failureInjector.GPUUUIDsWithGPULost,
			GPUUUIDsWithGPURequiresReset:                  op.failureInjector.GPUUUIDsWithGPURequiresReset,
			GPUUUIDsWithFabricStateHealthSummaryUnhealthy: op.failureInjector.GPUUUIDsWithFabricStateHealthSummaryUnhealthy,
			GPUProductNameOverride:                        op.failureInjector.GPUProductNameOverride,
		})
	} else {
		nvmlInstance, err = nvidianvml.New()
	}
	if err != nil {
		return err
	}

	mi, err := pkgmachineinfo.GetMachineInfo(nvmlInstance)
	if err != nil {
		return err
	}
	fmt.Printf("\n%s machine info\n", cmdcommon.CheckMark)
	mi.RenderTable(os.Stdout)

	if mi.GPUInfo != nil && mi.GPUInfo.Product != "" {
		threshold, err := nvidiainfiniband.SupportsInfinibandPortRate(mi.GPUInfo.Product)
		if err == nil {
			log.Logger.Infow("setting default expected port states", "product", mi.GPUInfo.Product, "at_least_ports", threshold.AtLeastPorts, "at_least_rate", threshold.AtLeastRate)
			nvidiainfiniband.SetDefaultExpectedPortStates(threshold)
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

	// Initialize all components first
	var initializedComponents []components.Component
	for _, c := range all.All() {
		c, err := c.InitFunc(gpudInstance)
		if err != nil {
			return err
		}
		if !c.IsSupported() {
			continue
		}
		initializedComponents = append(initializedComponents, c)
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
