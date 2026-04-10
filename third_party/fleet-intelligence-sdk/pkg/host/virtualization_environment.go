// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package host

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/file"
)

// VirtualizationEnvironment represents the virtualization environment of the host.
type VirtualizationEnvironment struct {
	// Type is the virtualization type.
	// Output of "systemd-detect-virt".
	// e.g., "kvm" for VM, "lxc" for container
	Type string `json:"type"`

	// Whether the host is running in a VM.
	// Output of "systemd-detect-virt --vm".
	// Set to "none" if the host is not running in a VM.
	// e.g., "kvm"
	VM string `json:"vm"`

	// Whether the host is running in a container.
	// Output of "systemd-detect-virt --container".
	// Set to "none" if the host is not running in a container.
	// e.g., "lxc"
	Container string `json:"container"`

	// Whether the host is running in a KVM.
	// Set to "false" if the host is not running in a KVM.
	IsKVM bool `json:"is_kvm"`
}

// GetSystemdDetectVirt detects the virtualization type of the host, using "systemd-detect-virt".
func GetSystemdDetectVirt(ctx context.Context) (VirtualizationEnvironment, error) {
	detectExecPath, err := file.LocateExecutable("systemd-detect-virt")
	if err != nil {
		return VirtualizationEnvironment{}, nil
	}

	vm, err := runSystemdDetectVirt(ctx, detectExecPath, "--vm")
	if err != nil {
		return VirtualizationEnvironment{}, err
	}

	container, err := runSystemdDetectVirt(ctx, detectExecPath, "--container")
	if err != nil {
		return VirtualizationEnvironment{}, err
	}

	virtType, err := runSystemdDetectVirt(ctx, detectExecPath)
	if err != nil {
		return VirtualizationEnvironment{}, err
	}

	virt := VirtualizationEnvironment{}
	virt.VM = vm
	virt.IsKVM = virt.VM == "kvm"
	virt.Container = container
	virt.Type = virtType
	return virt, nil
}

func runSystemdDetectVirt(ctx context.Context, detectExecPath string, args ...string) (string, error) {
	output, err := exec.CommandContext(ctx, detectExecPath, args...).CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err == nil {
		return trimmed, nil
	}
	if ctx.Err() != nil {
		return trimmed, ctx.Err()
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// systemd-detect-virt returns exit code 1 when no matching virtualization
		// environment is detected. Preserve the previous "|| true" behavior only
		// for that specific case and surface other execution failures.
		if exitErr.ExitCode() == 1 {
			return trimmed, nil
		}
	}

	return "", fmt.Errorf("failed to read systemd-detect-virt output: %w\n\noutput:\n%s", err, trimmed)
}

// GetSystemManufacturer detects the system manufacturer, using "dmidecode".
func GetSystemManufacturer(ctx context.Context) (string, error) {
	dmidecodePath, err := file.LocateExecutable("dmidecode")
	if err != nil {
		return "", nil
	}

	output, err := exec.CommandContext(ctx, "sudo", dmidecodePath, "-s", "system-manufacturer").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to read dmidecode for system manufacturer: %w\n\noutput:\n%s", err, strings.TrimSpace(string(output)))
	}

	return strings.TrimSpace(string(output)), nil
}
