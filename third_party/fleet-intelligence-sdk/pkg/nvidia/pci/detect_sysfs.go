// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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

package pci

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultSysfsPCIDevicesPath = "/sys/bus/pci/devices"

// DetectGPUHardware reports whether the host exposes an NVIDIA display
// controller. It reads PCI identity from sysfs first, which works directly on
// bare metal and through the host /sys mount used by the fleetint DaemonSet.
// lspci remains a fallback for environments where sysfs is unavailable.
func DetectGPUHardware(ctx context.Context) (bool, error) {
	found, err := detectGPUHardwareFromSysfs(defaultSysfsPCIDevicesPath)
	if err == nil {
		return found, nil
	}

	devices, lspciErr := ListPCIGPUs(ctx)
	if lspciErr == nil {
		return len(devices) > 0, nil
	}

	return false, errors.Join(
		fmt.Errorf("detect NVIDIA GPU from sysfs: %w", err),
		fmt.Errorf("detect NVIDIA GPU with lspci: %w", lspciErr),
	)
}

func detectGPUHardwareFromSysfs(root string) (bool, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		devicePath := filepath.Join(root, entry.Name())
		vendor, err := readPCIHexValue(filepath.Join(devicePath, "vendor"))
		if errors.Is(err, os.ErrNotExist) {
			// A PCI device can disappear while sysfs is being enumerated.
			continue
		}
		if err != nil {
			return false, err
		}
		if vendor != 0x10de {
			continue
		}

		class, err := readPCIHexValue(filepath.Join(devicePath, "class"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, err
		}
		if class>>16 == 0x03 {
			return true, nil
		}
	}

	return false, nil
}

func readPCIHexValue(path string) (uint64, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	value := strings.TrimSpace(string(contents))
	value = strings.TrimPrefix(strings.ToLower(value), "0x")
	parsed, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	return parsed, nil
}
