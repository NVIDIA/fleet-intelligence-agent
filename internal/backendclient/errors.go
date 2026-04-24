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
	"errors"
	"fmt"
	"strings"
)

// ErrNotImplemented is returned by skeleton backend client methods.
var ErrNotImplemented = errors.New("backend client not implemented")

// HTTPStatusError captures a non-2xx backend response.
type HTTPStatusError struct {
	StatusCode int
	Body       string
}

func (e *HTTPStatusError) Error() string {
	body := sanitizeErrorBody(e.Body)
	if body != "" {
		return fmt.Sprintf("backend request failed with status %d: %s", e.StatusCode, body)
	}
	return fmt.Sprintf("backend request failed with status %d", e.StatusCode)
}

func sanitizeErrorBody(body string) string {
	const maxLen = 200

	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	body = strings.ReplaceAll(body, "\n", " ")
	body = strings.ReplaceAll(body, "\r", " ")
	body = strings.Join(strings.Fields(body), " ")
	if len(body) <= maxLen {
		return body
	}
	return body[:maxLen] + "...(truncated)"
}
