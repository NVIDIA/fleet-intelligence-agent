#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2025 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

die() {
    echo "ERROR: $*" >&2
    exit 1
}

require_env() {
    local name="$1"
    if [ -z "${!name:-}" ]; then
        die "$name ENV variable must be provided"
    fi
}

require_env SKYHOOK_DIR

CONFIG_FILE="/etc/default/gpuhealth"
CONFIGMAP_FILE="${SKYHOOK_DIR}/configmaps/gpuhealth.config"

if [ ! -f "$CONFIG_FILE" ]; then
    die "configuration file $CONFIG_FILE does not exist"
fi

# If a configmap file exists, ensure its key/value pairs are reflected in /etc/default/gpuhealth
if [ -f "$CONFIGMAP_FILE" ]; then
    while IFS= read -r line || [ -n "$line" ]; do
        # skip empty lines and comments
        if [ -z "$line" ] || [[ "$line" =~ ^[[:space:]]*# ]]; then
            continue
        fi

        # only validate lines of the form VARIABLE=value
        if [[ "$line" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]]; then
            var="${line%%=*}"
            # Pull the effective line from the config file (first match)
            current="$(grep -m1 -E "^${var}=" "$CONFIG_FILE" || true)"

            if [ -z "$current" ]; then
                die "missing ${var} in ${CONFIG_FILE}"
            fi

            # Compare exact assignment string, but avoid printing values (may contain secrets)
            if [ "$current" != "$line" ]; then
                die "mismatched ${var} in ${CONFIG_FILE}"
            fi
        fi
    done < "$CONFIGMAP_FILE"
fi

# Basic runtime sanity checks
if [ ! -x "/usr/bin/gpuhealth" ]; then
    die "/usr/bin/gpuhealth is not present"
fi

if ! systemctl is-active --quiet gpuhealthd.service; then
    die "gpuhealthd.service is not running"
fi

if ! /usr/bin/gpuhealth status >/dev/null 2>&1; then
    die "gpuhealth status failed"
fi

exit 0
