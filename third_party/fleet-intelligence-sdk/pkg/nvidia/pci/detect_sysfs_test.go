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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectGPUHardwareFromSysfs(t *testing.T) {
	tests := []struct {
		name    string
		vendor  string
		class   string
		want    bool
		wantErr bool
	}{
		{name: "NVIDIA 3D controller", vendor: "0x10de", class: "0x030200", want: true},
		{name: "NVIDIA VGA controller", vendor: "0x10de", class: "0x030000", want: true},
		{name: "NVIDIA PCI bridge", vendor: "0x10de", class: "0x060400", want: false},
		{name: "non-NVIDIA display controller", vendor: "0x1002", class: "0x030200", want: false},
		{name: "invalid class", vendor: "0x10de", class: "invalid", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			devicePath := filepath.Join(root, "0000:01:00.0")
			require.NoError(t, os.Mkdir(devicePath, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(devicePath, "vendor"), []byte(tt.vendor+"\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(devicePath, "class"), []byte(tt.class+"\n"), 0o644))

			got, err := detectGPUHardwareFromSysfs(root)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetectGPUHardwareFromSysfsSkipsRemovedDevice(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Symlink(filepath.Join(root, "removed"), filepath.Join(root, "0000:01:00.0")))

	found, err := detectGPUHardwareFromSysfs(root)
	require.NoError(t, err)
	assert.False(t, found)
}
