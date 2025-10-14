# Installation

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
| RHEL | 9, 10 | x86_64, ARM64 |
| Rocky Linux | 9, 10 | x86_64, ARM64 |
| AlmaLinux | 9, 10 | x86_64, ARM64 |
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

