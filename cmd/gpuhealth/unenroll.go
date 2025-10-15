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

package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/urfave/cli"

	"github.com/NVIDIA/gpuhealth/internal/config"
)

func unenrollCommand(c *cli.Context) error {
	log.Logger.Infow("Starting un-enrollment process")

	// Get state file path
	stateFile, err := config.DefaultStateFile()
	if err != nil {
		return fmt.Errorf("failed to get state file path: %w", err)
	}

	// Open database connection
	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state database: %w", err)
	}
	defer dbRW.Close()

	// Remove enrollment metadata entries
	if err := removeEnrollmentMetadata(context.Background(), dbRW); err != nil {
		return fmt.Errorf("failed to remove enrollment metadata: %w", err)
	}

	log.Logger.Infow("Successfully un-enrolled from GPU Health backend")
	return nil
}

// removeEnrollmentMetadata removes all enrollment-related metadata from the database
func removeEnrollmentMetadata(ctx context.Context, dbRW *sql.DB) error {
	// List of metadata keys to clear (set to empty string)
	keysToClear := []string{
		pkgmetadata.MetadataKeyToken,
		"sak_token",
		"enroll_endpoint",
		"metrics_endpoint",
		"logs_endpoint",
		"nonce_endpoint",
	}

	// Clear each metadata entry by setting it to empty string
	for _, key := range keysToClear {
		if err := pkgmetadata.SetMetadata(ctx, dbRW, key, ""); err != nil {
			log.Logger.Errorw("Failed to clear metadata key", "key", key, "error", err)
			return fmt.Errorf("failed to clear metadata key %s: %w", key, err)
		} else {
			log.Logger.Infow("Cleared metadata key", "key", key)
		}
	}

	return nil
}
