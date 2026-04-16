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

package attestation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

var execCommandContext = exec.CommandContext

type cliEvidenceCollector struct {
	timeout time.Duration
}

// NewCLIEvidenceCollector creates an evidence collector backed by the nvattest CLI.
func NewCLIEvidenceCollector(timeout time.Duration) EvidenceCollector {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &cliEvidenceCollector{timeout: timeout}
}

func (c *cliEvidenceCollector) Collect(ctx context.Context, nonce string) (*SDKResponse, error) {
	if err := validateNonce(nonce); err != nil {
		return nil, fmt.Errorf("invalid nonce received from backend: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := execCommandContext(
		runCtx,
		"nvattest",
		"collect-evidence",
		"--gpu-evidence-source=corelib",
		"--nonce", nonce,
		"--gpu-architecture", "blackwell",
		"--format", "json",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil && stdout.Len() == 0 {
		return nil, fmt.Errorf("attestation CLI execution failed: %w (stderr: %s)", err, stderr.String())
	}

	var response SDKResponse
	if parseErr := json.Unmarshal(stdout.Bytes(), &response); parseErr != nil {
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		return nil, fmt.Errorf(
			"failed to parse CLI response: %w (stderr: %s), stdout: %s, error: %s",
			parseErr, stderr.String(), stdout.String(), errText,
		)
	}
	return &response, nil
}

func validateNonce(nonce string) error {
	if nonce == "" {
		return fmt.Errorf("nonce is empty")
	}
	const maxLen = 512
	if len(nonce) > maxLen {
		return fmt.Errorf("nonce length %d exceeds maximum of %d characters", len(nonce), maxLen)
	}
	for i, c := range nonce {
		switch {
		case c >= '0' && c <= '9',
			c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c == '-', c == '_', c == '=', c == '+', c == '/':
		default:
			return fmt.Errorf("nonce contains invalid character %q at position %d", c, i)
		}
	}
	return nil
}
