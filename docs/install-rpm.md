# RPM Installation

## Prerequisites

The Fleet Intelligence Agent RPM package has the following runtime dependencies:

- `datacenter-gpu-manager-4-proprietary` (DCGM)
- `nvattest` (NVIDIA Attestation SDK CLI, NVAT)
- `corelib` (NVAT GPU evidence source dependency)

Before installing Fleet Intelligence Agent, ensure the following prerequisites are met:

- NVIDIA package repository is configured (network or local CUDA repository) so `datacenter-gpu-manager-4-proprietary`, `nvattest`, and `corelib` can be installed
- DCGM HostEngine `4.2.3` or newer
- NVIDIA Datacenter Driver major version `510` or newer is installed
- Install/upgrade commands are run as `root` or with `sudo`
- `dnf-plugins-core` is installed (required for `dnf config-manager`)
- Attestation for the fleetint use case only supports Blackwell and newer GPUs, and applies to non-CC mode systems
- For NVSwitch systems (driver branch must match installed datacenter driver):
  - Hopper (pre-4th gen NVSwitch): install `nvidia-driver:<driver-branch>/fm`
  - Blackwell (4th gen NVSwitch): install `nvidia-driver-<driver-branch>-open` and `nvlink-<driver-branch>`

References:
- DCGM: <https://docs.nvidia.com/datacenter/dcgm/latest/user-guide/getting-started.html#installation>
- Fabric Manager: <https://docs.nvidia.com/datacenter/tesla/fabric-manager-user-guide/index.html#installing-fabric-manager>
- NVAT (`nvattest`/`corelib`): <https://docs.nvidia.com/attestation/nv-attestation-sdk-cpp/latest/overview.html>

Fleet Intelligence Agent package dependencies (`datacenter-gpu-manager-4-proprietary`, `nvattest`, and `corelib`) are available through NVIDIA's CUDA repository. Before installing Fleet Intelligence Agent, add the NVIDIA CUDA repository for your system.

Install `dnf-plugins-core` first if `dnf config-manager` is not available:

```bash
sudo dnf install -y dnf-plugins-core
```

### RHEL/Rocky/AlmaLinux Systems

```bash
. /etc/os-release
ARCH=$(uname -m)
URL_ARCH=$([ "$ARCH" = "aarch64" ] && echo "sbsa" || echo "$ARCH")
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/rhel${VERSION_ID%%.*}/${URL_ARCH}/cuda-rhel${VERSION_ID%%.*}.repo
sudo dnf clean all
```

### Amazon Linux 2023

```bash
ARCH=$(uname -m)
URL_ARCH=$([ "$ARCH" = "aarch64" ] && echo "sbsa" || echo "$ARCH")
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/amzn2023/${URL_ARCH}/cuda-amzn2023.repo
sudo dnf clean all
```

After adding the CUDA repository, package dependencies (`datacenter-gpu-manager-4-proprietary`, `nvattest`, and `corelib`) are resolved automatically by `dnf` during Fleet Intelligence Agent installation.

## Install package

```bash
VERSION=$(curl -fsSL https://api.github.com/repos/NVIDIA/fleet-intelligence-agent/releases/latest \
  | grep '"tag_name"' | cut -d '"' -f 4 | tr -d 'v')
ARCH=$(uname -m)

curl -fLO https://github.com/NVIDIA/fleet-intelligence-agent/releases/download/v${VERSION}/fleetint-${VERSION}-1.${ARCH}.rpm
sudo dnf install ./fleetint-${VERSION}-1.${ARCH}.rpm
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
ARCH=$(uname -m)

curl -fLO https://github.com/NVIDIA/fleet-intelligence-agent/releases/download/v${VERSION}/fleetint-${VERSION}-1.${ARCH}.rpm
sudo dnf install ./fleetint-${VERSION}-1.${ARCH}.rpm
```

The service will automatically restart with the new version.

## Uninstall

```bash
sudo dnf remove fleetint
```
