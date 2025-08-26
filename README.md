# GPUHealth

[![Go Report Card](https://goreportcard.com/badge/github.com/NVIDIA/gpuhealth)](https://goreportcard.com/report/github.com/NVIDIA/gpuhealth)
![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/NVIDIA/gpuhealth?sort=semver)
[![Go Reference](https://pkg.go.dev/badge/github.com/NVIDIA/gpuhealth.svg)](https://pkg.go.dev/github.com/NVIDIA/gpuhealth)

## Overview

**GPUHealth** is a streamlined GPU health monitoring and reporting tool designed to ensure GPU reliability by actively monitoring GPU status and exporting health metrics for analysis.

## About GPUHealth

GPUHealth is based on the upstream [leptonai/gpud](https://github.com/leptonai/gpud) project but focuses specifically on GPU health monitoring without management overhead. It is built on years of experience operating large-scale GPU clusters and is carefully designed to be self-contained with seamless integration into existing monitoring infrastructure.

### Key Characteristics

- **Health-Focused**: Concentrates purely on GPU health monitoring and metrics export
- **Lightweight**: Self-contained binary with minimal CPU and memory footprint  
- **Non-Intrusive**: Operates with read-only operations in a non-critical path
- **Integration-Ready**: Easy to integrate with existing monitoring and alerting systems
- **Production-Ready**: Built for reliability in datacenter environments

### Architecture

GPUHealth operates as a standalone monitoring agent that:
- Collects GPU health metrics and status information
- Detects hardware issues and performance anomalies  
- Exports data in standard formats (JSON, CSV)
- Supports both online (HTTP endpoint) and offline (file-based) modes

## Get Started

### Quick Start

To quickly check your GPU health status:

```bash
# Download and run a quick scan
gpuhealth scan
```

### Installation

#### From GitHub Releases

Download the latest release for your platform from [GitHub Releases](https://github.com/NVIDIA/gpuhealth/releases):

```bash
# Example for Linux x86_64
wget https://github.com/NVIDIA/gpuhealth/releases/latest/download/gpuhealth_linux_amd64.tar.gz
tar -xzf gpuhealth_linux_amd64.tar.gz
sudo mv gpuhealth /usr/local/bin/
```

#### Package Installation

**Debian/Ubuntu:**
```bash
# Download .deb package
wget https://github.com/NVIDIA/gpuhealth/releases/latest/download/gpuhealth_amd64.deb
sudo dpkg -i gpuhealth_amd64.deb
```

**RHEL/CentOS:**
```bash
# Download .rpm package  
wget https://github.com/NVIDIA/gpuhealth/releases/latest/download/gpuhealth-x86_64.rpm
sudo rpm -i gpuhealth-x86_64.rpm
```

#### Build from Source

```bash
git clone https://github.com/NVIDIA/gpuhealth.git
cd gpuhealth
make gpuhealth
sudo mv bin/gpuhealth /usr/local/bin/
```

### Usage

#### Health Monitoring Server

Start the health monitoring server:

```bash
# Start server (runs on port 15133 by default)  
gpuhealth run

# Start with custom configuration
gpuhealth run --listen-address=0.0.0.0:8080 --log-level=debug
```

#### One-time Health Check

Perform a quick health scan:

```bash
gpuhealth scan
```

#### Offline Data Collection

Collect health data to files:

```bash
# Collect data for 1 hour to /tmp/gpu-health/
gpuhealth run --offline-mode --path=/tmp/gpu-health --duration=1h
```

#### Check Service Status

```bash
gpuhealth status
```

### API Access

Once running, access health data via HTTP API:

```bash
# Health endpoint
curl http://localhost:15133/healthz

# Machine information  
curl http://localhost:15133/machine-info

# Health states
curl http://localhost:15133/v1/states
```

## Key Features

### GPU Health Monitoring
- **Hardware Metrics**: Power consumption, temperature, clock speeds, utilization
- **Error Detection**: NVML Xid events, hardware slowdown, row remapping failures  
- **Fabric Health**: GPU fabric status and interconnect monitoring
- **Performance Tracking**: GPU performance counters and throughput metrics

### System Health Monitoring  
- **Basic System Metrics**: CPU, memory, and disk usage
- **Driver Status**: NVIDIA driver version and compatibility checks
- **Process Monitoring**: GPU process information and resource allocation

### Data Export & Integration
- **Multiple Formats**: JSON and CSV output formats
- **HTTP API**: RESTful endpoints for real-time data access
- **Offline Mode**: File-based data collection for batch processing
- **Configurable Intervals**: Customizable health check frequencies

### Production Features
- **Low Overhead**: Minimal CPU and memory footprint
- **Read-Only**: Non-intrusive monitoring with no system modifications
- **Reliability**: Built for 24/7 operation in datacenter environments
- **Scalability**: Deploy across large GPU clusters with consistent performance

Check out [*components*](./docs/COMPONENTS.md) for a detailed list of monitoring components and their capabilities.

## FAQs

### Does GPUHealth send data externally?

**No.** GPUHealth operates in a fully self-contained mode and does not send any data to external services by default. All health monitoring data is:

- Stored locally on your system
- Accessed only through the local HTTP API (if enabled)  
- Exported to local files in offline mode
- **Never transmitted** to external services without explicit configuration

GPUHealth is designed for environments where data privacy and security are paramount.

### How do I integrate GPUHealth with my monitoring system?

GPUHealth provides multiple integration options:

**HTTP API Integration:**
```bash
# Prometheus-style metrics
curl http://localhost:15133/metrics

# JSON health data
curl http://localhost:15133/v1/states
```

**File-based Integration:**
```bash
# Export to files for processing
gpuhealth run --offline-mode --path=/monitoring/data --duration=24h
```

**Custom Endpoints:**
Configure your monitoring system to scrape the GPUHealth API endpoints at your desired interval.

### How do I update GPUHealth?

1. **Download latest release** from [GitHub Releases](https://github.com/NVIDIA/gpuhealth/releases)
2. **Stop running instance**: `gpuhealth status` to check, then stop if needed
3. **Replace binary**: Update `/usr/local/bin/gpuhealth` or your installation path
4. **Restart**: Launch gpuhealth with your previous configuration

For package installations (deb/rpm), use your system's package manager to update.

### What are the system requirements?

- **OS**: Linux (primary support), basic support for other Unix-like systems
- **Architecture**: x86_64 (amd64), ARM64 (aarch64)  
- **NVIDIA Driver**: Version 535+ recommended (not required for basic system monitoring)
- **Memory**: ~10-50MB RAM usage
- **CPU**: Minimal overhead, typically <1% CPU usage
- **Storage**: ~100MB for binary and logs

### Can I run GPUHealth without NVIDIA drivers?

Yes! GPUHealth will operate in a reduced functionality mode:
- ✅ **System monitoring**: CPU, memory, disk metrics still available
- ✅ **Basic GPU detection**: PCI device enumeration  
- ❌ **NVIDIA-specific monitoring**: Requires NVIDIA drivers for full GPU health data

## Documentation

- [Components Guide](./docs/COMPONENTS.md) - Detailed component information and configuration
- [Architecture Overview](./docs/ARCHITECTURE.md) - System design and technical details  
- [Installation Guide](./docs/INSTALL.md) - Comprehensive installation instructions
- [Integration Guide](./docs/INTEGRATION.md) - How to integrate with monitoring systems

## Related Projects

- **Upstream Project**: [leptonai/gpud](https://github.com/leptonai/gpud) - Full-featured GPU management daemon
- **NVIDIA Tools**: Compatible with NVIDIA's GPU monitoring ecosystem

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for:
- Development setup and build instructions
- Code style and contribution guidelines  
- How to report issues and submit pull requests
- Upstream sync procedures for maintainers

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
