# NVIDIA GPU Health Monitoring Agent

## Overview

`gpuhealth` is a lightweight GPU health monitoring agent that tracks GPU status and exports health metrics. Based on [leptonai/gpud](https://github.com/leptonai/gpud), it focuses specifically on monitoring without management overhead.

**Key Features:**
- **Health-Focused**: GPU health monitoring and metrics export  
- **Lightweight**: Minimal CPU and memory footprint (<100MB RAM, <1% CPU)
- **Non-Intrusive**: Read-only operations, no system modifications
- **Integration-Ready**: HTTP API, file export, optional centralized reporting  
- **Production-Ready**: Built for 24/7 datacenter operation

## Quick Start

### Installation

**Package Installation (Recommended):**
```bash
# Ubuntu/Debian
wget https://github.com/NVIDIA/gpuhealth/releases/latest/download/gpuhealth_*_amd64.deb
sudo dpkg -i gpuhealth_*_amd64.deb

# RHEL/Rocky/AlmaLinux/AmazonLinux
wget https://github.com/NVIDIA/gpuhealth/releases/latest/download/gpuhealth-*-1.x86_64.rpm
sudo rpm -i gpuhealth-*-1.x86_64.rpm

# Verify installation
systemctl status gpuhealthd
```

**Build from Source:**
```bash
git clone https://github.com/NVIDIA/gpuhealth.git
cd gpuhealth  
make gpuhealth
sudo mv bin/gpuhealth /usr/local/bin/
```

### Usage

```bash
# Start monitoring server (port 15133)
gpuhealth run

# Quick health check
gpuhealth scan  

# Offline data collection
gpuhealth run --offline-mode --path=/tmp/gpu-health --duration=00:05:00 --format csv

# Check status
gpuhealth status
```

### API Access

```bash
# Health status
curl http://localhost:15133/healthz

# Machine info & health states  
curl http://localhost:15133/machine-info
curl http://localhost:15133/v1/states

# Prometheus metrics
curl http://localhost:15133/metrics
```

## What It Monitors

- **GPU Health**: Power, temperature, clocks, utilization, Xid events
- **System Metrics**: CPU, memory, disk usage  
- **Driver Status**: NVIDIA driver version and compatibility
- **Process Info**: GPU process allocation and resource usage

## Data Export

- **HTTP API**: Real-time JSON/Prometheus metrics
- **Offline Mode**: File-based data collection (JSON/CSV)
- **Centralized Reporting**: Optional push to control planes

See [Components Guide](./docs/COMPONENTS.md) for detailed monitoring capabilities.

## FAQ

**Does it send data externally?**  
No, by default all data stays local. Optional centralized reporting can be configured if desired.

**System requirements?**  
Ubuntu 22.04+, RHEL 8+, <100MB RAM, <1% CPU. NVIDIA drivers recommended but not required.

**Integration options?**  
HTTP API (JSON/Prometheus), offline file export, or optional push to monitoring systems.

## Documentation

- [Components Guide](./docs/COMPONENTS.md) - Monitoring capabilities and configuration
- [Architecture Overview](./docs/ARCHITECTURE.md) - System design and technical details  
- [Installation Guide](./docs/INSTALL.md) - Comprehensive setup instructions
- [Integration Guide](./docs/INTEGRATION.md) - Monitoring system integration

## Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

**Related Projects:** [leptonai/gpud](https://github.com/leptonai/gpud) (upstream full-featured GPU management daemon)

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.
