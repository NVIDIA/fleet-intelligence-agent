# RPM Installation

## Prerequisites: NVIDIA CUDA Repository

The Fleet Intelligence Agent requires NVIDIA DCGM (Data Center GPU Manager) for GPU monitoring. DCGM is available through NVIDIA's CUDA repository and will be automatically installed when you install the Fleet Intelligence Agent package.

Before installing the Fleet Intelligence Agent, add the appropriate NVIDIA CUDA repository for your system.

### RHEL/Rocky/AlmaLinux Systems

```bash
# RHEL/Rocky/AlmaLinux 8 (x86_64)
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/rhel8/x86_64/cuda-rhel8.repo
sudo dnf clean all

# RHEL/Rocky/AlmaLinux 9 (x86_64)
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/rhel9/x86_64/cuda-rhel9.repo
sudo dnf clean all

# RHEL/Rocky/AlmaLinux 10 (x86_64)
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/rhel10/x86_64/cuda-rhel10.repo
sudo dnf clean all

# RHEL/Rocky/AlmaLinux 8 (ARM64)
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/rhel8/sbsa/cuda-rhel8.repo
sudo dnf clean all

# RHEL/Rocky/AlmaLinux 9 (ARM64)
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/rhel9/sbsa/cuda-rhel9.repo
sudo dnf clean all

# RHEL/Rocky/AlmaLinux 10 (ARM64)
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/rhel10/sbsa/cuda-rhel10.repo
sudo dnf clean all
```

### Amazon Linux 2023

```bash
# Amazon Linux 2023 (x86_64)
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/amzn2023/x86_64/cuda-amzn2023.repo
sudo dnf clean all

# Amazon Linux 2023 (ARM64)
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/amzn2023/sbsa/cuda-amzn2023.repo
sudo dnf clean all
```

After adding the CUDA repository, DCGM (`datacenter-gpu-manager-4-core`) will be automatically installed as a recommended dependency when you install the Fleet Intelligence Agent package.

## Install package

Download the package from [Releases](https://github.com/NVIDIA/fleet-intelligence-agent/releases), then install:

```bash
# RHEL/Rocky/AlmaLinux/Amazon Linux (x86_64)
sudo dnf install ./fleetint-VERSION-1.x86_64.rpm

# RHEL/Rocky/AlmaLinux/Amazon Linux (ARM64)
sudo dnf install ./fleetint-VERSION-1.aarch64.rpm
```

Verify:

```bash
fleetint --version
systemctl status fleetintd
```

## Update

Install the newer package version:

```bash
# RHEL/Rocky/AlmaLinux/Amazon Linux (x86_64)
sudo dnf install ./fleetint-VERSION-1.x86_64.rpm

# RHEL/Rocky/AlmaLinux/Amazon Linux (ARM64)
sudo dnf install ./fleetint-VERSION-1.aarch64.rpm
```

The service will automatically restart with the new version.

## Uninstall

```bash
sudo dnf remove fleetint
```

