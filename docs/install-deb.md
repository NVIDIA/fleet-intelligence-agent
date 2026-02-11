# DEB Installation

## Prerequisites: NVIDIA CUDA Repository

The GPU Health Agent requires NVIDIA DCGM (Data Center GPU Manager) for GPU monitoring. DCGM is available through NVIDIA's CUDA repository and will be automatically installed when you install the GPU Health Agent package.

Before installing the GPU Health Agent, add the appropriate NVIDIA CUDA repository for your system:

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

After adding the CUDA repository, DCGM (`datacenter-gpu-manager-4-core`) will be automatically installed as a recommended dependency when you install the GPU Health Agent package.

## Install package

Download the package from [Releases](https://github.com/NVIDIA/gpuhealth/releases), then install:

```bash
# Ubuntu (x86_64)
sudo apt install ./gpuhealth_VERSION_amd64.deb

# Ubuntu (ARM64)
sudo apt install ./gpuhealth_VERSION_arm64.deb
```

Verify:

```bash
gpuhealth --version
systemctl status gpuhealthd
```

## Update

Install the newer package version:

```bash
# Ubuntu (x86_64)
sudo apt install ./gpuhealth_VERSION_amd64.deb

# Ubuntu (ARM64)
sudo apt install ./gpuhealth_VERSION_arm64.deb
```

The service will automatically restart with the new version.

## Uninstall

```bash
sudo apt remove gpuhealth
sudo apt purge gpuhealth  # Also removes configuration files
```

