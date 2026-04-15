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

package agentstate

import (
	"context"
	"database/sql"
	"fmt"

	pkgmetadata "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metadata"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/sqlite"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/config"
)

const metadataKeyBackendBaseURL = "backend_base_url"

type sqliteState struct {
	stateFileFn func() (string, error)
}

// NewSQLite returns a State backed by the agent sqlite metadata database.
func NewSQLite() State {
	return &sqliteState{stateFileFn: config.DefaultStateFile}
}

func (s *sqliteState) GetBackendBaseURL(ctx context.Context) (string, bool, error) {
	return s.getMetadata(ctx, metadataKeyBackendBaseURL)
}

func (s *sqliteState) SetBackendBaseURL(ctx context.Context, value string) error {
	return s.setMetadata(ctx, metadataKeyBackendBaseURL, value)
}

func (s *sqliteState) GetJWT(ctx context.Context) (string, bool, error) {
	return s.getMetadata(ctx, pkgmetadata.MetadataKeyToken)
}

func (s *sqliteState) SetJWT(ctx context.Context, value string) error {
	return s.setMetadata(ctx, pkgmetadata.MetadataKeyToken, value)
}

func (s *sqliteState) GetSAK(ctx context.Context) (string, bool, error) {
	return s.getMetadata(ctx, "sak_token")
}

func (s *sqliteState) SetSAK(ctx context.Context, value string) error {
	return s.setMetadata(ctx, "sak_token", value)
}

func (s *sqliteState) GetNodeID(ctx context.Context) (string, bool, error) {
	return s.getMetadata(ctx, pkgmetadata.MetadataKeyMachineID)
}

func (s *sqliteState) SetNodeID(ctx context.Context, value string) error {
	return s.setMetadata(ctx, pkgmetadata.MetadataKeyMachineID, value)
}

func (s *sqliteState) getMetadata(ctx context.Context, key string) (string, bool, error) {
	db, err := s.openReadOnly()
	if err != nil {
		return "", false, err
	}
	defer db.Close()

	value, err := pkgmetadata.ReadMetadata(ctx, db, key)
	if err != nil {
		return "", false, fmt.Errorf("read metadata %q: %w", key, err)
	}
	if value == "" {
		return "", false, nil
	}
	return value, true, nil
}

func (s *sqliteState) setMetadata(ctx context.Context, key, value string) error {
	db, err := s.openReadWrite()
	if err != nil {
		return err
	}
	defer db.Close()

	if err := pkgmetadata.CreateTableMetadata(ctx, db); err != nil {
		return fmt.Errorf("create metadata table: %w", err)
	}
	if err := pkgmetadata.SetMetadata(ctx, db, key, value); err != nil {
		return fmt.Errorf("set metadata %q: %w", key, err)
	}
	stateFile, err := s.stateFileFn()
	if err == nil {
		if err := config.SecureStateFilePermissions(stateFile); err != nil {
			return fmt.Errorf("secure state file permissions: %w", err)
		}
	}
	return nil
}

func (s *sqliteState) openReadOnly() (*sql.DB, error) {
	stateFile, err := s.stateFileFn()
	if err != nil {
		return nil, fmt.Errorf("get state file path: %w", err)
	}
	db, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return nil, fmt.Errorf("open state database read-only: %w", err)
	}
	return db, nil
}

func (s *sqliteState) openReadWrite() (*sql.DB, error) {
	stateFile, err := s.stateFileFn()
	if err != nil {
		return nil, fmt.Errorf("get state file path: %w", err)
	}
	db, err := sqlite.Open(stateFile)
	if err != nil {
		return nil, fmt.Errorf("open state database: %w", err)
	}
	return db, nil
}
