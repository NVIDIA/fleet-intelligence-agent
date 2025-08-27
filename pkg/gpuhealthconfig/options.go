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

package gpuhealthconfig

import (
	pkgconfigcommon "github.com/leptonai/gpud/pkg/config/common"
	infinibandclass "github.com/leptonai/gpud/pkg/nvidia-query/infiniband/class"
)

// Op contains options for health configuration
type Op struct {
	pkgconfigcommon.ToolOverwrites
}

// OpOption is a function that modifies health configuration options
type OpOption func(*Op)

// ApplyOpts applies all the provided options to the Op struct
func (op *Op) ApplyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.InfinibandClassRootDir == "" {
		op.InfinibandClassRootDir = infinibandclass.DefaultRootDir
	}

	return nil
}

// WithInfinibandClassRootDir specifies the root directory of the InfiniBand class
func WithInfinibandClassRootDir(p string) OpOption {
	return func(op *Op) {
		op.InfinibandClassRootDir = p
	}
}
