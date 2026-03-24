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

package dcgmversion

import (
	"os"
	"strconv"
	"strings"

	godcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
)

var getenv = os.Getenv

// DetectHostengineVersion returns the DCGM HostEngine version for the current
// environment. It initializes a standalone DCGM connection and extracts the
// semantic version from the build info string.
func DetectHostengineVersion() (string, error) {
	initParams := resolveInitParams()
	cleanup, err := godcgm.Init(godcgm.Standalone, initParams.address, initParams.isUnixSocket)
	if err != nil {
		return "", err
	}
	defer cleanup()

	versionInfo, err := godcgm.GetHostengineVersionInfo()
	if err != nil {
		return "", err
	}

	return extractVersion(versionInfo.RawBuildInfoString), nil
}

type initParams struct {
	address      string
	isUnixSocket string
}

func resolveInitParams() initParams {
	address := strings.TrimSpace(getenv("DCGM_URL"))
	isUnixSocket := "0"

	if truthy, err := strconv.ParseBool(strings.TrimSpace(getenv("DCGM_URL_IS_UNIX_SOCKET"))); err == nil && truthy {
		isUnixSocket = "1"
	}

	if address == "" {
		address = "localhost"
	}

	return initParams{
		address:      address,
		isUnixSocket: isUnixSocket,
	}
}

func extractVersion(raw string) string {
	for _, pair := range strings.Split(raw, ";") {
		key, value, ok := strings.Cut(pair, ":")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) == "version" {
			return strings.TrimSpace(value)
		}
	}

	return ""
}
