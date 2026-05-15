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

package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/cmdutil"
	agenttag "github.com/NVIDIA/fleet-intelligence-agent/internal/tag"
)

func unsetTagCommand(cliContext *cli.Context) error {
	clearNodeGroup := cliContext.Bool("nodegroup")
	clearComputeZone := cliContext.Bool("compute-zone")
	rawCustomKeys := cliContext.StringSlice("tag")
	if !clearNodeGroup && !clearComputeZone && len(rawCustomKeys) == 0 {
		return fmt.Errorf("at least one target must be provided: --nodegroup, --compute-zone, or --tag key")
	}

	customKeys := make([]string, 0, len(rawCustomKeys))
	for _, rawKey := range rawCustomKeys {
		key, err := agenttag.NormalizeAndValidateKey(rawKey)
		if err != nil {
			return fmt.Errorf("invalid --tag %q: %w", rawKey, err)
		}
		if agenttag.IsReservedKey(key) {
			return fmt.Errorf("reserved key %q must use --nodegroup or --compute-zone", key)
		}
		customKeys = append(customKeys, key)
	}

	clearPatch := map[string]string{}
	if clearNodeGroup {
		clearPatch[agenttag.ReservedKeyNodeGroup] = ""
	}
	if clearComputeZone {
		clearPatch[agenttag.ReservedKeyComputeZone] = ""
	}
	for _, key := range customKeys {
		clearPatch[key] = ""
	}
	if err := agenttag.ValidateReservedPairPatch(clearPatch); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), tagCommandTimeout)
	defer cancel()

	state := newTagCommandState()
	existing, ok, err := state.GetTags(ctx)
	if err != nil {
		return fmt.Errorf("read existing tags: %w", err)
	}
	if !ok {
		existing = map[string]string{}
	}

	if clearNodeGroup {
		delete(existing, agenttag.ReservedKeyNodeGroup)
	}
	if clearComputeZone {
		delete(existing, agenttag.ReservedKeyComputeZone)
	}
	for _, key := range customKeys {
		delete(existing, key)
	}

	if err := state.SetTags(ctx, existing); err != nil {
		return fmt.Errorf("persist tags: %w", err)
	}
	if err := upsertTagsAfterTagUpdate(ctx, clearPatch); err != nil {
		return fmt.Errorf("tags were updated locally but backend upsert failed: %w", err)
	}
	fmt.Printf("%s successfully unset tags\n", cmdutil.CheckMark)
	return nil
}
