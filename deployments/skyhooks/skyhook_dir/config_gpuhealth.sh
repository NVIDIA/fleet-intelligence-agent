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
set -x

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

# check if config file exists
if [ ! -f "$CONFIG_FILE" ]; then
    die "configuration file $CONFIG_FILE does not exist"
fi

# process configMap file if it exists
CONFIGMAP_FILE="${SKYHOOK_DIR}/configmaps/gpuhealth.config"
if [ -f "$CONFIGMAP_FILE" ]; then
    echo "Processing configuration from $CONFIGMAP_FILE"
    
    # read the configmap file line by line
    while IFS= read -r line || [ -n "$line" ]; do
        # skip empty lines and comments
        if [ -z "$line" ] || [[ "$line" =~ ^[[:space:]]*# ]]; then
            continue
        fi
        
        # check if line has the format VARIABLE=value
        if [[ "$line" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]]; then
            # extract the variable name (everything before the first =)
            VARIABLE_NAME="${line%%=*}"

            # Upsert safely (avoid sed escaping issues if values contain &, |, etc.)
            TMP_FILE="$(mktemp)"
            awk -v var="$VARIABLE_NAME" -v newline="$line" '
                BEGIN { found=0 }
                $0 ~ "^" var "=" {
                    if (!found) {
                        print newline
                        found=1
                    }
                    next
                }
                { print }
                END { if (!found) print newline }
            ' "$CONFIG_FILE" > "$TMP_FILE"

            if ! cmp -s "$CONFIG_FILE" "$TMP_FILE"; then
                echo "Updating $VARIABLE_NAME in $CONFIG_FILE from configmap"
                mv "$TMP_FILE" "$CONFIG_FILE"
            else
                rm -f "$TMP_FILE"
            fi
        fi
    done < "$CONFIGMAP_FILE"
    
    echo "ConfigMap processing complete"
else
    echo "No configMap file found at $CONFIGMAP_FILE, skipping"
fi

# restart gpuhealthd service to apply changes
systemctl restart gpuhealthd.service
