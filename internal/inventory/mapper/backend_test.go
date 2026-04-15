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

package mapper

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/inventory"
)

func TestToNodeUpsertRequestNil(t *testing.T) {
	require.Nil(t, ToNodeUpsertRequest(nil))
}

func TestToNodeUpsertRequest(t *testing.T) {
	req := ToNodeUpsertRequest(&inventory.Snapshot{
		NodeID:                  "node-1",
		Hostname:                "host-a",
		MachineID:               "machine-id",
		SystemUUID:              "uuid-1",
		BootID:                  "boot-1",
		OperatingSystem:         "linux",
		OSImage:                 "Ubuntu",
		KernelVersion:           "6.5.0",
		FleetintVersion:         "1.2.3",
		GPUDriverVersion:        "550.54.15",
		CUDAVersion:             "12.4",
		DCGMVersion:             "4.2.3",
		ContainerRuntimeVersion: "containerd://1.7.13",
		NetPrivateIP:            "10.0.0.10",
		NetPublicIP:             "203.0.113.10",
		InventoryHash:           "hash-1",
		Resources: inventory.Resources{
			CPUInfo: inventory.CPUInfo{
				Type:         "Xeon",
				Manufacturer: "Intel",
				Architecture: "x86_64",
				LogicalCores: 64,
			},
			MemoryInfo: inventory.MemoryInfo{
				TotalBytes: 1024,
			},
			GPUInfo: inventory.GPUInfo{
				Product:      "H100",
				Manufacturer: "NVIDIA",
				Architecture: "Hopper",
				Memory:       "80GB",
				GPUs: []inventory.GPUDevice{{
					UUID:         "GPU-1",
					BusID:        "0000:01:00.0",
					SN:           "serial",
					MinorID:      "1",
					BoardID:      7,
					VBIOSVersion: "vbios",
					ChassisSN:    "chassis",
					GPUIndex:     "0",
				}},
			},
			DiskInfo: inventory.DiskInfo{
				ContainerRootDisk: "/dev/nvme0n1",
				BlockDevices: []inventory.BlockDevice{{
					Name:       "nvme0n1",
					Type:       "disk",
					Size:       2048,
					WWN:        "wwn",
					MountPoint: "/",
					FSType:     "ext4",
					PartUUID:   "part-uuid",
					Parents:    []string{"parent0"},
				}},
			},
			NICInfo: inventory.NICInfo{
				PrivateIPInterfaces: []inventory.NICInterface{{
					Interface: "eth0",
					MAC:       "00:11:22:33:44:55",
					IP:        "10.0.0.10",
				}},
			},
		},
	})

	require.NotNil(t, req)
	require.Equal(t, "host-a", req.Hostname)
	require.Equal(t, "machine-id", req.MachineID)
	require.Equal(t, "203.0.113.10", req.NetPublicIP)
	require.Equal(t, "hash-1", req.InventoryHash)
	require.Equal(t, int64(64), req.Resources.CPUInfo.LogicalCores)
	require.Equal(t, uint64(1024), req.Resources.MemoryInfo.TotalBytes)
	require.Len(t, req.Resources.GPUInfo.GPUs, 1)
	require.Equal(t, 7, req.Resources.GPUInfo.GPUs[0].BoardID)
	require.Len(t, req.Resources.DiskInfo.BlockDevices, 1)
	require.Equal(t, "parent0", req.Resources.DiskInfo.BlockDevices[0].Parents[0])
	require.Len(t, req.Resources.NICInfo.PrivateIPInterfaces, 1)
	require.Equal(t, "eth0", req.Resources.NICInfo.PrivateIPInterfaces[0].Interface)
}
