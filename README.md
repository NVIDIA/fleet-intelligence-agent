# NVIDIA GPU Health Agent

## Overview

`gpuhealth` is a lightweight GPU health monitoring and reporting agent for NVIDIA GPU infrastructure. Built on [leptonai/gpud](https://github.com/leptonai/gpud), it provides focused health monitoring without management overhead.

**Key Features:**
- **Lightweight**: <100MB RAM, <1% CPU usage
- **Non-intrusive**: Read-only operations, no system modifications
- **Flexible**: HTTP API, Prometheus metrics, CSV/JSON export
- **Production-ready**: 24/7 datacenter operation

## Quick Start

### Prerequisites

- Linux (Ubuntu 22.04+, RHEL 9+, Amazon Linux 2023+)
- Go 1.24+ (for building from source)
- NVIDIA drivers (recommended)
- Root/sudo access

### Installation

#### Package Installation (Recommended)

**Step 1: Download Package**
1. Go to the [Releases page](https://github.com/NVIDIA/gpuhealth/releases)
2. Download the appropriate package for your system:
   - **Debian/Ubuntu x86_64**: `gpuhealth_VERSION_amd64.deb`
   - **Debian/Ubuntu ARM64**: `gpuhealth_VERSION_arm64.deb`
   - **RHEL/RockyLinux/AlmaLinux/AmazonLinux x86_64**: `gpuhealth-VERSION-1.x86_64.rpm`
   - **RHEL/RockyLinux/AlmaLinux/AmazonLinux ARM64**: `gpuhealth-VERSION-1.aarch64.rpm`

**Step 2: Install Package**
```bash
# Debian/Ubuntu (x86_64)
sudo apt install ./gpuhealth_VERSION_amd64.deb

# Debian/Ubuntu (ARM64)
sudo apt install ./gpuhealth_VERSION_arm64.deb

# RHEL/RockyLinux/AlmaLinux/AmazonLinux (x86_64)
sudo dnf install ./gpuhealth-VERSION-1.x86_64.rpm

# RHEL/RockyLinux/AlmaLinux/AmazonLinux (ARM64)
sudo dnf install ./gpuhealth-VERSION-1.aarch64.rpm
```

**Step 3: Verify Installation**
```bash
# Verify installation
gpuhealth --version
systemctl status gpuhealthd
```

#### Binary Installation (Alternative)

For systems where you prefer not to use packages (e.g. offline mode):

1. Download the binary archive for your platform from the [Releases page](https://github.com/NVIDIA/gpuhealth/releases)
2. Extract and install:
```bash
# Download and extract (replace VERSION and ARCH as needed)
tar -xzf gpuhealth_vVERSION_linux_ARCH.tar.gz
sudo cp gpuhealth /usr/local/bin/

# Verify installation
gpuhealth --version
```

**Build from Source (Alternative Method):**
```bash
# Clone the repository
git clone https://github.com/NVIDIA/gpuhealth.git
cd gpuhealth

# Build the binary
make gpuhealth

# Install system-wide (optional)
sudo cp bin/gpuhealth /usr/local/bin/

# Verify installation
./bin/gpuhealth --version
```

## Development Setup

### Prerequisites for Development

- Go 1.24+ 
- Git with SSH access to NVIDIA internal repositories
- Access to `gitlab-master.nvidia.com:12051`

### Clone and Setup

```bash
# Clone the repository
git clone https://github.com/NVIDIA/gpuhealth.git
cd gpuhealth

# Configure Go for private NVIDIA GitLab access
go env -w GOPRIVATE=gitlab-master.nvidia.com/* GONOPROXY=gitlab-master.nvidia.com/* GONOSUMDB=gitlab-master.nvidia.com/*

# Configure Git to use SSH for GitLab (if using SSH keys)
git config --global url."ssh://git@gitlab-master.nvidia.com:12051/".insteadOf "https://gitlab-master.nvidia.com/"

# Download dependencies
go mod tidy

# Build and test
make all && make test && make lint
```

### Usage

```bash
# Quick health scan
gpuhealth scan

# Start monitoring server (HTTP API on port 15133)
gpuhealth run

# Check status and machine info
gpuhealth status
gpuhealth machine-info

# Offline data collection
gpuhealth run --offline-mode --path=/tmp/gpu-health --duration=00:05:00 --format=csv
```

### HTTP API

```bash
# Health check
curl http://localhost:15133/healthz

# Machine and GPU info
curl http://localhost:15133/machine-info

# Current health states
curl http://localhost:15133/v1/states

# Prometheus metrics
curl http://localhost:15133/metrics
```

## What It Monitors

**GPU Metrics:** Power, temperature, clocks, utilization, memory, Xid events  
**System Metrics:** CPU, memory, disk, network usage  
**Infrastructure:** NVIDIA drivers, CUDA runtime, InfiniBand, containers

## Export Formats

- **HTTP API**: JSON, Prometheus metrics
- **File Export**: CSV, JSON, OTLP

## FAQ

**Q: Does it send data externally?**  
A: No, all data stays local by default, can be configured to send to an OTLP compatible endpoint.

**Q: System requirements?**  
A: Linux, <100MB RAM, <1% CPU. NVIDIA drivers recommended but not required.

**Q: Works without NVIDIA GPUs?**  
A: Yes, system monitoring works without GPUs.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

**Related:** [leptonai/gpud](https://github.com/leptonai/gpud) (upstream dependency)

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.
