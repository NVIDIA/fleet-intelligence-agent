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
	"path/filepath"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/sqlite"
	"github.com/stretchr/testify/require"
)

func TestReadEnrollmentStatusMissingMetadataTable(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "state.db"))
	require.NoError(t, err)
	defer db.Close()

	status, err := readEnrollmentStatus(ctx, db)
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Empty(t, status.baseURL)
	require.Empty(t, status.metricsEndpoint)
	require.Empty(t, status.logsEndpoint)
}
