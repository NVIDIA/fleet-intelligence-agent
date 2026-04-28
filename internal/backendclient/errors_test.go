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

package backendclient

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPStatusErrorError(t *testing.T) {
	t.Parallel()

	err := (&HTTPStatusError{StatusCode: 500, Body: "line1\nline2"}).Error()
	require.Contains(t, err, "status 500")
	require.Contains(t, err, "line1 line2")

	err = (&HTTPStatusError{StatusCode: 404}).Error()
	require.Equal(t, "backend request failed with status 404", err)
}

func TestSanitizeErrorBody(t *testing.T) {
	t.Parallel()

	require.Empty(t, sanitizeErrorBody(" \n\t "))
	require.Equal(t, "hello world", sanitizeErrorBody(" hello \n world "))

	body := strings.Repeat("a", 220)
	got := sanitizeErrorBody(body)
	require.True(t, strings.HasSuffix(got, "...(truncated)"))
	require.LessOrEqual(t, len(got), 214)
}
