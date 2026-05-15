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
	"time"

	"github.com/urfave/cli"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/agentstate"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/enrollment"
	agenttag "github.com/NVIDIA/fleet-intelligence-agent/internal/tag"
)

const tagCommandTimeout = 2 * time.Minute

var (
	upsertTagsAfterTagUpdate = enrollment.UpsertTagsNow
	newTagCommandState       = agentstate.NewSQLite
)

func tagCommand(cliContext *cli.Context) error {
	args := make([]string, 0, cliContext.NArg())
	for i := 0; i < cliContext.NArg(); i++ {
		args = append(args, cliContext.Args().Get(i))
	}
	updates, err := agenttag.ParseCLIArgs(args)
	if err != nil {
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
	merged := agenttag.Clone(existing)
	for key, value := range updates {
		merged[key] = value
	}
	if err := state.SetTags(ctx, merged); err != nil {
		return fmt.Errorf("persist tags: %w", err)
	}

	if err := upsertTagsAfterTagUpdate(ctx, updates); err != nil {
		return fmt.Errorf("tags were saved but backend upsert failed: %w", err)
	}
	fmt.Printf("%s successfully updated tags\n", cmdutil.CheckMark)
	return nil
}
