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

// Package nodeidentity initializes and persists the stable node UUID used by
// enrollment, inventory, attestation, and metrics workflows.
// The persisted node UUID is stored in the legacy machine ID metadata key.
package nodeidentity

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/prometheus/procfs/sysfs"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
)

// State is the local storage contract needed to persist node identity.
type State interface {
	GetOrCreateNodeUUID(ctx context.Context, create func() (string, error)) (value string, created bool, err error)
}

var (
	readHardwareUUID = readDMIProductUUID
	newNodeUUID      = uuid.NewString
)

// EnsureNodeUUID returns the persisted node UUID, or initializes and persists it
// from DMI sysfs with a random UUID fallback.
func EnsureNodeUUID(ctx context.Context, state State) (string, error) {
	if state == nil {
		return "", errors.New("node identity state is required")
	}

	nodeUUID, created, err := state.GetOrCreateNodeUUID(ctx, func() (string, error) {
		nodeUUID, err := readHardwareUUID()
		if err != nil || nodeUUID == "" {
			nodeUUID = newNodeUUID()
			log.Logger.Warnw("Failed to get hardware UUID, generated random agent ID",
				"error", err,
				"generated_id", nodeUUID)
		} else {
			log.Logger.Infow("Initialized agent ID from hardware UUID", "machine_id", nodeUUID)
		}
		return nodeUUID, nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to ensure node UUID: %w", err)
	}
	if created {
		log.Logger.Infow("Persisted agent ID to database", "machine_id", nodeUUID)
	} else {
		log.Logger.Infow("Using persisted agent ID from database", "machine_id", nodeUUID)
	}
	return nodeUUID, nil
}

func readDMIProductUUID() (string, error) {
	return readDMIProductUUIDFromFS("/sys")
}

func readDMIProductUUIDFromFS(root string) (string, error) {
	fs, err := sysfs.NewFS(root)
	if err != nil {
		return "", fmt.Errorf("failed to open sysfs: %w", err)
	}

	dmi, err := fs.DMIClass()
	if err != nil {
		return "", fmt.Errorf("failed to read DMI class: %w", err)
	}
	if dmi.ProductUUID == nil {
		return "", errors.New("DMI product UUID not found")
	}

	nodeUUID := strings.TrimSpace(*dmi.ProductUUID)
	if nodeUUID == "" {
		return "", errors.New("DMI product UUID is empty")
	}
	parsedUUID, err := uuid.Parse(nodeUUID)
	if err != nil {
		return "", fmt.Errorf("invalid DMI product UUID %q: %w", nodeUUID, err)
	}
	if parsedUUID == uuid.Nil {
		return "", errors.New("DMI product UUID is unset")
	}
	return nodeUUID, nil
}
