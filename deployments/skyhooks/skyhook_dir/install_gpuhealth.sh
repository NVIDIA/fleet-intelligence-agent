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

have_cmd() {
    command -v "$1" >/dev/null 2>&1
}

require_env GPUHEALTH_VERSION
require_env GPUHEALTH_ENDPOINT
require_env GPUHEALTH_TOKEN
require_env SKYHOOK_DIR

# figure out the distro by sourcing the os-release file
source /etc/os-release
case "${ID:-}" in
    ubuntu*)
        DISTRO_TYPE="deb"
        ;;
    # Supported RPM-family distros per docs/installation.md (RHEL/Rocky/AlmaLinux, Amazon Linux 2023, Alinux 3)
    rhel* | rocky* | almalinux* | amzn* | alinux*)
        DISTRO_TYPE="rpm"
        ;;
    *)
        die "unsupported distro: ${ID:-unknown}"
        ;;
esac

# figure out the CPU architecture
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)
        if [ "$DISTRO_TYPE" = "rpm" ]; then
            ARCH="x86_64"
        else
            ARCH="amd64"
        fi
    ;;
    aarch64)
        if [ "$DISTRO_TYPE" = "rpm" ]; then
            ARCH="aarch64"
        else
            ARCH="arm64"
        fi
    ;;
    *)
        die "unsupported architecture: $ARCH"
    ;;
esac

# gpuhealth is packaged with different field separators based on the distro type for rpm distros it is - and for deb distros it is _
# use the above information to construct the packages name gpuhealth then version then architecture then . distro type
if [ "$DISTRO_TYPE" = "rpm" ]; then
    SEPARATOR="-"
    PACKAGE_NAME="gpuhealth${SEPARATOR}${GPUHEALTH_VERSION}${SEPARATOR}1.${ARCH}.${DISTRO_TYPE}"
else
    SEPARATOR="_"
    PACKAGE_NAME="gpuhealth${SEPARATOR}${GPUHEALTH_VERSION}${SEPARATOR}${ARCH}.${DISTRO_TYPE}"
fi

resolve_package_path() {
    local pkg_name="$1"
    local local_path="${SKYHOOK_DIR}/skyhook_dir/${pkg_name}"

    if [ -f "$local_path" ]; then
        echo "$local_path"
        return 0
    fi

    die "package not found at ${local_path}"
}

PKG_PATH="$(resolve_package_path "$PACKAGE_NAME")"

# install the gpuhealth package (prefer high-level package managers for dependency resolution)
case "$DISTRO_TYPE" in
    deb)
        export DEBIAN_FRONTEND=noninteractive
        if have_cmd apt-get; then
            # Installing via apt-get resolves dependencies better than dpkg -i
            apt-get install -y "$PKG_PATH" || (dpkg -i "$PKG_PATH" && apt-get -f install -y)
        else
            dpkg -i "$PKG_PATH"
        fi
        ;;
    rpm)
        if have_cmd dnf; then
            dnf install -y "$PKG_PATH"
        elif have_cmd yum; then
            yum install -y "$PKG_PATH"
        else
            # Fallback: low-level install/upgrade without dependency resolution
            rpm -Uvh "$PKG_PATH"
        fi
        ;;
    *)
        die "unsupported distro type: $DISTRO_TYPE"
        ;;
esac

# Do this without setting -x to avoid logging the token
set +x
if /usr/bin/gpuhealth enroll --endpoint "$GPUHEALTH_ENDPOINT" --token "$GPUHEALTH_TOKEN"; then
    echo "SUCCESS: gpuhealth enrollment completed successfully"
else
    echo "ERROR: gpuhealth enrollment failed with exit code $?"
    set -x
    exit 1
fi
set -x

# enable and start gpuhealthd service
systemctl enable --now gpuhealthd.service
