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

have_cmd() {
    command -v "$1" >/dev/null 2>&1
}

is_pkg_installed() {
    local name="$1"
    if [ "$DISTRO_TYPE" = "deb" ]; then
        dpkg-query -W -f='${Status}' "$name" 2>/dev/null | grep -q "install ok installed"
        return $?
    fi
    rpm -q "$name" >/dev/null 2>&1
}

# figure out the distro by sourcing the os-release file
source /etc/os-release
case "${ID:-}" in
    ubuntu*)
        DISTRO_TYPE="deb"
    ;;
    # Supported RPM-family distros per docs/installation.md (RHEL/Rocky/AlmaLinux, Amazon Linux 2023)
    # and deployments/skyhooks/README.md (Alibaba Linux).
    rhel* | rocky* | almalinux* | amzn* | alinux*)
        DISTRO_TYPE="rpm"
    ;;
    *)
        die "unsupported distro: ${ID:-unknown}"
    ;;
esac

if [ "$DISTRO_TYPE" = "deb" ] && ! have_cmd dpkg-query; then
    die "dpkg-query is not available"
fi
if [ "$DISTRO_TYPE" = "rpm" ] && ! have_cmd rpm; then
    die "rpm is not available"
fi

# unenroll (best effort)
if [ -x "/usr/bin/gpuhealth" ]; then
    /usr/bin/gpuhealth unenroll || true
fi

# disable and stop gpuhealthd service (best effort)
if have_cmd systemctl; then
    if systemctl list-unit-files --type=service 2>/dev/null | awk '{print $1}' | grep -qx "gpuhealthd.service"; then
        systemctl disable --now gpuhealthd.service || true
    fi
fi

# uninstall the gpuhealth package
case $DISTRO_TYPE in
    deb)
        export DEBIAN_FRONTEND=noninteractive
        if is_pkg_installed gpuhealth; then
            if have_cmd apt-get; then
                apt-get remove -y gpuhealth || true
                apt-get autoremove -y || true
            else
                # fallback if apt-get isn't present
                apt remove -y gpuhealth || true
                apt autoremove -y || true
            fi
        else
            echo "gpuhealth package is not installed; skipping removal"
        fi
    ;;
    rpm)
        if is_pkg_installed gpuhealth; then
            if have_cmd dnf; then
                dnf remove -y gpuhealth || true
            elif have_cmd yum; then
                yum remove -y gpuhealth || true
            else
                rpm -e gpuhealth || true
            fi
        else
            echo "gpuhealth package is not installed; skipping removal"
        fi
    ;;
    *)
        die "unsupported distro type: $DISTRO_TYPE"
    ;;
esac

