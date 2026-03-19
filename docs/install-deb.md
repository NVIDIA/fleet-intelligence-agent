# DEB Installation

## Prerequisites

The Fleet Intelligence Agent DEB package has the following runtime dependencies:

- `datacenter-gpu-manager-4-proprietary` (DCGM)
- `nvattest` (NVIDIA Attestation SDK CLI, NVAT)
- `corelib` (NVAT GPU evidence source dependency)

Before installing Fleet Intelligence Agent, ensure the following prerequisites are met:

- NVIDIA package repository is configured (network or local CUDA repository) so `datacenter-gpu-manager-4-proprietary`, `nvattest`, and `corelib` can be installed
- DCGM HostEngine `4.2.3` or newer
- NVIDIA Datacenter Driver major version `510` or newer is installed
- Install/upgrade commands are run as `root` or with `sudo`
- Attestation for the fleetint use case only supports Blackwell and newer GPUs, and applies to non-CC mode systems
- For NVSwitch systems (driver branch must match installed datacenter driver):
  - Hopper (pre-4th gen NVSwitch): install `cuda-drivers-fabricmanager-<driver-branch>`
  - Blackwell (4th gen NVSwitch): install `nvidia-open-<driver-branch>` and `nvlink5-<driver-branch>`

References:
- DCGM: <https://docs.nvidia.com/datacenter/dcgm/latest/user-guide/getting-started.html#installation>
- Fabric Manager: <https://docs.nvidia.com/datacenter/tesla/fabric-manager-user-guide/index.html#installing-fabric-manager>
- NVAT (`nvattest`/`corelib`): <https://docs.nvidia.com/attestation/nv-attestation-sdk-cpp/latest/overview.html>

Fleet Intelligence Agent package dependencies (`datacenter-gpu-manager-4-proprietary`, `nvattest`, and `corelib`) are available through NVIDIA's CUDA repository. Before installing Fleet Intelligence Agent, add the NVIDIA CUDA repository for your system:

```bash
. /etc/os-release
DISTRO="${ID}${VERSION_ID//./}"
ARCH=$(dpkg --print-architecture)
URL_ARCH=$([ "$ARCH" = "arm64" ] && echo "sbsa" || echo "$ARCH")
curl -fLO https://developer.download.nvidia.com/compute/cuda/repos/${DISTRO}/${URL_ARCH}/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb
sudo apt-get update
```

After adding the CUDA repository, package dependencies (`datacenter-gpu-manager-4-proprietary`, `nvattest`, and `corelib`) are resolved automatically by `apt` during Fleet Intelligence Agent installation.

## Install package

```bash
VERSION=$(curl -fsSL https://api.github.com/repos/NVIDIA/fleet-intelligence-agent/releases/latest \
  | grep '"tag_name"' | cut -d '"' -f 4 | tr -d 'v')
ARCH=$(dpkg --print-architecture)

curl -fLO https://github.com/NVIDIA/fleet-intelligence-agent/releases/download/v${VERSION}/fleetint_${VERSION}_${ARCH}.deb
sudo apt install ./fleetint_${VERSION}_${ARCH}.deb
```

To install a specific version, set `VERSION` explicitly instead (e.g., `VERSION=0.2.0`).

Verify:

```bash
fleetint --version
systemctl status fleetintd
```

## Update

Set `VERSION` to the target version and reinstall:

```bash
VERSION=<new-version>
ARCH=$(dpkg --print-architecture)

curl -fLO https://github.com/NVIDIA/fleet-intelligence-agent/releases/download/v${VERSION}/fleetint_${VERSION}_${ARCH}.deb
sudo apt install ./fleetint_${VERSION}_${ARCH}.deb
```

The service will automatically restart with the new version.

## Uninstall

```bash
sudo apt remove fleetint
sudo apt purge fleetint  # Also removes configuration files
```
