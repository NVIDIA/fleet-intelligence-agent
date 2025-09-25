# NVIDIA GPU Health Agent

## Overview

Lightweight GPU health monitoring and reporting agent for NVIDIA GPU infrastructure building on top of [leptonai/gpud](https://github.com/leptonai/gpud)

**What It Monitors:**
- **GPU Metrics**: Power, temperature, clocks, utilization, memory, Xid events
- **System Metrics**: CPU, memory, disk, network usage
- **Infrastructure**: NVIDIA drivers, CUDA runtime, InfiniBand, containers

**Export Formats:**
- **HTTP API Server**: Serves data via REST endpoints (JSON) and Prometheus metrics (`/metrics`)
- **File Export (Offline Mode)**: Writes data to local files in CSV or JSON format
- **Remote Export**: Sends telemetry data to OpenTelemetry-compatible endpoints via OTLP over HTTP

**Key Features:**
- **Lightweight**: <100MB RAM, <1% CPU usage
- **Non-intrusive**: Read-only operations, no system modifications
- **Production-ready**: 24/7 datacenter operation

## Quick Start

### Installation

#### Package Installation (Recommended)

Download the appropriate package from [Releases](https://github.com/NVIDIA/gpuhealth/releases):

```bash
# Debian/Ubuntu (x86_64)
sudo apt install ./gpuhealth_VERSION_amd64.deb

# Debian/Ubuntu (ARM64)
sudo apt install ./gpuhealth_VERSION_arm64.deb

# RHEL/Rocky/AlmaLinux/Amazon Linux (x86_64)
sudo dnf install ./gpuhealth-VERSION-1.x86_64.rpm

# RHEL/Rocky/AlmaLinux/Amazon Linux (ARM64)
sudo dnf install ./gpuhealth-VERSION-1.aarch64.rpm

# Verify
gpuhealth --version
systemctl status gpuhealthd
```

#### Binary Installation

```bash
# Download and extract binary
tar -xzf gpuhealth_vVERSION_linux_ARCH.tar.gz
sudo cp gpuhealth /usr/local/bin/
gpuhealth --version
```

#### Build from Source

```bash
git clone https://github.com/NVIDIA/gpuhealth.git
cd gpuhealth
make gpuhealth
sudo cp bin/gpuhealth /usr/local/bin/
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

### Configuration

Edit `/etc/default/gpuhealth` and restart the service:

```bash
sudo nano /etc/default/gpuhealth
sudo systemctl restart gpuhealthd
```

Register for remote export:

```bash
sudo gpuhealth register --endpoint "https://telemetry.company.com/v1" --token "your-token"
sudo systemctl restart gpuhealthd
```

HTTP proxy configuration:

```bash
# Add to /etc/default/gpuhealth
HTTP_PROXY="http://proxy.company.com:8080"
HTTPS_PROXY="http://proxy.company.com:8080"

# For authenticated proxies
HTTP_PROXY="http://username:password@proxy.company.com:8080"
HTTPS_PROXY="http://username:password@proxy.company.com:8080"
```

### Available Configuration Options

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `GPUHEALTH_FLAGS` | Additional command line flags | `--log-level=warn` |
| `GPUHEALTH_COLLECT_INTERVAL` | Data collection interval | `1m` |
| `GPUHEALTH_INCLUDE_METRICS` | Include metrics in export | `true` |
| `GPUHEALTH_INCLUDE_EVENTS` | Include events in export | `true` |
| `GPUHEALTH_INCLUDE_MACHINEINFO` | Include machine info | `true` |
| `GPUHEALTH_INCLUDE_HEALTHCHECKS` | Include component health data | `true` |
| `GPUHEALTH_METRICS_LOOKBACK` | How far back to look for metrics | `1m` |
| `GPUHEALTH_EVENTS_LOOKBACK` | How far back to look for events | `1m` |
| `GPUHEALTH_CHECK_INTERVAL` | Component health check frequency | `1m` |
| `GPUHEALTH_RETRY_MAX_ATTEMPTS` | Max retry attempts for failed exports | `3` |
| `HTTP_PROXY` | HTTP proxy server URL | - |
| `HTTPS_PROXY` | HTTPS proxy server URL | - |

## Development

```bash
# Clone and setup
git clone https://github.com/NVIDIA/gpuhealth.git
cd gpuhealth

# Configure for NVIDIA internal access
go env -w GOPRIVATE=gitlab-master.nvidia.com/*

# Build and test
make all && make test
```

## FAQ

**Q: Does it send data externally?**  
A: No, all data stays local by default, can be configured to send to an OTLP compatible endpoint.

**Q: System requirements?**  
A: Linux, <100MB RAM, <1% CPU. NVIDIA drivers recommended but not required.

**Q: Works without NVIDIA GPUs?**  
A: Yes, system monitoring works without GPUs.

**Q: How do I troubleshoot export issues?**
A: Check logs with `sudo journalctl -u gpuhealthd -f` and verify endpoints with `sudo gpuhealth metadata`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

**Related:** [leptonai/gpud](https://github.com/leptonai/gpud) (upstream dependency)

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.
