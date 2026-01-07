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

have_cmd() {
    command -v "$1" >/dev/null 2>&1
}

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

if [ "$DISTRO_TYPE" = "deb" ]; then
    if ! have_cmd dpkg-query; then
        die "dpkg-query is not available"
    fi
    if ! dpkg-query -W -f='${Status}' gpuhealth 2>/dev/null | grep -q "install ok installed"; then
        die "gpuhealth package is not installed."
    fi
else
    if ! have_cmd rpm; then
        die "rpm is not available"
    fi
    if ! rpm -q gpuhealth >/dev/null 2>&1; then
        die "gpuhealth package is not installed."
    fi
fi

# ensure that gpuhealth files are present
if [ ! -f "/usr/bin/gpuhealth" ]; then
    die "gpuhealth files are not present."
fi

# ensure system service is present
if [ ! -f "/usr/lib/systemd/system/gpuhealthd.service" ]; then
    die "gpuhealth service is not present."
fi

# ensure system service is enabled
if ! systemctl is-enabled --quiet gpuhealthd.service; then
    die "gpuhealth service is not enabled."
fi

# ensure system service is running
if ! systemctl is-active --quiet gpuhealthd.service; then
    die "gpuhealth service is not running."
fi

# ensure gpuhealth status command works
if ! /usr/bin/gpuhealth status >/dev/null 2>&1; then
    die "gpuhealth status failed."
fi
