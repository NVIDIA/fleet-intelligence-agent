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

# package should be absent
if [ "$DISTRO_TYPE" = "deb" ]; then
    if have_cmd dpkg-query; then
        if dpkg-query -W -f='${Status}' gpuhealth 2>/dev/null | grep -q "install ok installed"; then
            die "gpuhealth package is still installed"
        fi
    fi
else
    if have_cmd rpm; then
        if rpm -q gpuhealth >/dev/null 2>&1; then
            die "gpuhealth package is still installed"
        fi
    fi
fi

# binary should be absent (or at least not executable)
if [ -x "/usr/bin/gpuhealth" ]; then
    die "/usr/bin/gpuhealth still present"
fi

# service should be stopped/disabled if systemd is present
if have_cmd systemctl; then
    if systemctl list-unit-files --type=service 2>/dev/null | awk '{print $1}' | grep -qx "gpuhealthd.service"; then
        if systemctl is-active --quiet gpuhealthd.service; then
            die "gpuhealthd.service is still running"
        fi
        if systemctl is-enabled --quiet gpuhealthd.service; then
            die "gpuhealthd.service is still enabled"
        fi
    fi
fi

exit 0


