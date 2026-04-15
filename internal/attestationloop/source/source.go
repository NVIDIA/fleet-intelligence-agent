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

// Package source contains attestation loop collection adapters.
package source

import (
	"context"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/attestationloop"
)

// NVAttestCollector is the local attestation evidence collector dependency.
type NVAttestCollector interface {
	Collect(ctx context.Context, nonce string) (*attestationloop.SDKResponse, error)
}
