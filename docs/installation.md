# Installation

## Prerequisites: NVIDIA CUDA Repository

The GPU Health Agent requires NVIDIA DCGM (Data Center GPU Manager) for GPU monitoring. DCGM is available through NVIDIA's CUDA repository and will be automatically installed when you install the GPU Health Agent package.

**Before installing the GPU Health Agent**, add the appropriate NVIDIA CUDA repository for your system:

### Ubuntu/Debian Systems

```bash
# Ubuntu 22.04 (x86_64)
wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/x86_64/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb
sudo apt-get update

# Ubuntu 24.04 (x86_64)
wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2404/x86_64/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb
sudo apt-get update

# Ubuntu 22.04 (ARM64)
wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/sbsa/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb
sudo apt-get update

# Ubuntu 24.04 (ARM64)
wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2404/sbsa/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb
sudo apt-get update
```

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

**Note**: After adding the CUDA repository, DCGM (`datacenter-gpu-manager-4-core`) will be automatically installed as a recommended dependency when you install the GPU Health Agent package.

## Package Installation (Recommended)

Download the appropriate package from [Releases](https://github.com/NVIDIA/gpuhealth/releases):

```bash
# Ubuntu (x86_64)
sudo apt install ./gpuhealth_VERSION_amd64.deb

# Ubuntu (ARM64)
sudo apt install ./gpuhealth_VERSION_arm64.deb

# RHEL/Rocky/AlmaLinux/Amazon Linux (x86_64)
sudo dnf install ./gpuhealth-VERSION-1.x86_64.rpm

# RHEL/Rocky/AlmaLinux/Amazon Linux (ARM64)
sudo dnf install ./gpuhealth-VERSION-1.aarch64.rpm

# Verify
gpuhealth --version
systemctl status gpuhealthd
```

## Binary Installation

```bash
# Download and extract binary
tar -xzf gpuhealth_vVERSION_linux_ARCH.tar.gz
sudo cp gpuhealth /usr/local/bin/
gpuhealth --version
```

## Build from Source

```bash
git clone https://github.com/NVIDIA/gpuhealth.git
cd gpuhealth
make gpuhealth
sudo cp bin/gpuhealth /usr/local/bin/
```

## System Requirements

### Supported Operating Systems

| OS Family | Supported Versions | Architecture |
|-----------|-------------------|--------------|
| Ubuntu | 22.04, 24.04 | x86_64, ARM64 |
| RHEL | 8, 9, 10 | x86_64, ARM64 |
| Rocky Linux | 8, 9, 10 | x86_64, ARM64 |
| AlmaLinux | 8, 9, 10 | x86_64, ARM64 |
| Amazon Linux | 2023 | x86_64, ARM64 |

**Note**: The agent may run on other Linux distributions, but only the distributions listed above are officially supported and tested. Compatibility with other distributions is not guaranteed.

### Supported GPU Architectures

| Architecture | Type | Examples |
|--------------|------|----------|
| GPU | Hopper | H100, H200, GH200 |
| GPU | Blackwell | GB200, GB300, B200, B300 |

**NVIDIA Driver**: 535+

**Note**: Other GPU architectures and earlier driver versions may work but are not guaranteed. The agent uses NVML (NVIDIA Management Library) which is included with the NVIDIA driver. System monitoring features work without GPUs.

### Resource Requirements

- **Memory**: <100MB RAM
- **CPU**: <1% CPU usage
- **Network**: Optional, for remote export features

## Updating

Simply install the new package version:

```bash
# Ubuntu (x86_64)
sudo apt install ./gpuhealth_VERSION_amd64.deb

# Ubuntu (ARM64)
sudo apt install ./gpuhealth_VERSION_arm64.deb

# RHEL/Rocky/AlmaLinux/Amazon Linux (x86_64)
sudo dnf install ./gpuhealth-VERSION-1.x86_64.rpm

# RHEL/Rocky/AlmaLinux/Amazon Linux (ARM64)
sudo dnf install ./gpuhealth-VERSION-1.aarch64.rpm
```

The service will automatically restart with the new version.

## Uninstalling

```bash
# Ubuntu
sudo apt remove gpuhealth
sudo apt purge gpuhealth  # Also removes configuration files

# RHEL/Rocky/AlmaLinux
sudo dnf remove gpuhealth
```

