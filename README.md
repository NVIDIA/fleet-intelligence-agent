# NVIDIA GPU Health Monitoring and Reporting Agent

## Overview

**GPUHealth** is a streamlined GPU health monitoring and reporting tool designed to ensure GPU reliability by actively monitoring GPU status and exporting health metrics for analysis. **GPUHealth** is based on the upstream [leptonai/gpud](https://github.com/leptonai/gpud) project but focuses specifically on GPU health monitoring without management overhead.

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
- Supports multiple deployment modes:
  - **Local API**: HTTP endpoints for on-demand access
  - **Offline Collection**: File-based batch data export  
  - **Centralized Reporting**: Optional push-mode to control planes (configurable)

## Get Started

### Installation

Choose between **package installation** (recommended for production) or **building from source** (for development/customization):

#### Package Installation (Recommended)

**Includes systemd integration and auto-start capability**

**Debian/Ubuntu:**
```bash
# Download and install .deb package
wget https://github.com/NVIDIA/gpuhealth/releases/latest/download/gpuhealth_amd64.deb
sudo dpkg -i gpuhealth_amd64.deb

# Check the gpuhealthd service status
systemctl status gpuhealthd
```

**RHEL/CentOS:**
```bash
# Download and install .rpm package  
wget https://github.com/NVIDIA/gpuhealth/releases/latest/download/gpuhealth-x86_64.rpm
sudo rpm -i gpuhealth-x86_64.rpm

# Check the gpuhealthd service status
systemctl status gpuhealthd
```

**Package installation provides:**
- ✅ **Systemd integration**: Service management with `systemctl`
- ✅ **Auto-start**: Automatically starts on system boot
- ✅ **Service configuration**: Pre-configured service files and environment
- ✅ **Standard paths**: Binary, logs, and data stored in standard system locations

#### Build from Source

**For development, customization, or manual deployment**

```bash
git clone https://github.com/NVIDIA/gpuhealth.git
cd gpuhealth
make gpuhealth
sudo mv bin/gpuhealth /usr/local/bin/

# Manual setup required:
# - No systemd integration (run manually or create your own service)  
# - No auto-start capability
# - Manual configuration of paths and permissions
```

**Source installation provides:**
- ✅ **Latest code**: Access to newest features and bug fixes
- ✅ **Customization**: Modify source code as needed
- ✅ **Minimal installation**: Just the binary, no additional system integration
- ❌ **Manual setup**: You handle service management, auto-start, and configuration

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
- **Centralized Reporting**: Optional push-mode data export to control planes
- **Configurable Intervals**: Customizable health check and export frequencies
- **Flexible Endpoints**: Support for custom monitoring infrastructure integration

### Production Features
- **Low Overhead**: Minimal CPU and memory footprint
- **Read-Only**: Non-intrusive monitoring with no system modifications
- **Reliability**: Built for 24/7 operation in datacenter environments
- **Scalability**: Deploy across large GPU clusters with consistent performance

Check out [*components*](./docs/COMPONENTS.md) for a detailed list of monitoring components and their capabilities.

## FAQs

### Does GPUHealth send data externally?

**By default, no.** GPUHealth operates in a fully self-contained mode and does not send any data to external services by default. However, it **can be configured** to send health data to a centralized control plane for further analysis if desired.

**Default behavior:**
- Stored locally on your system
- Accessed only through the local HTTP API (if enabled)  
- Exported to local files in offline mode
- **No external data transmission** without explicit configuration

**Optional centralized reporting:**
- Can be configured to send health data to a centralized monitoring platform
- Configurable endpoints, intervals, and data filtering
- All data transmission is **opt-in** and under your control
- Supports secure channels for data transmission

GPUHealth is designed for environments where data privacy and security are paramount, giving you full control over where and how your GPU health data is used.

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

**Centralized Control Plane Integration:**
```bash
# Configure centralized reporting (optional)
gpuhealth run --health-exporter-endpoint=https://monitoring.company.com/gpu-health \
              --health-exporter-interval=5m \
              --include-metrics=true
```

**Custom Endpoints:**
Configure your monitoring system to scrape the GPUHealth API endpoints at your desired interval, or set up centralized reporting to push data to your monitoring infrastructure.

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
