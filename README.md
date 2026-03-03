# NVIDIA Fleet Intelligence Agent

Lightweight Fleet Intelligence monitoring and reporting agent for NVIDIA GPU infrastructure building on top of [leptonai/gpud](https://github.com/leptonai/gpud)

## Overview

**Prerequisites:**
- NVIDIA DCGM (Data Center GPU Manager) - automatically installed from NVIDIA CUDA repositories
- See [DEB Installation](docs/install-deb.md) or [RPM Installation](docs/install-rpm.md) for CUDA repository setup instructions

**What It Monitors:**
- GPU Metrics: Power, temperature, clocks, utilization, memory, Xid events
- System Metrics: CPU, memory, disk, network usage
- Infrastructure: NVIDIA drivers, CUDA runtime, InfiniBand, containers

**Export Formats:**
- HTTP API Server: Serves data via REST endpoints (JSON) and Prometheus metrics (`/metrics`)
- File Export (Offline Mode): Writes data to local files in CSV or JSON format
- Remote Export: Sends telemetry data to OpenTelemetry-compatible endpoints via OTLP over HTTP

**Key Features:**
- Lightweight: <100MB RAM, <1% CPU usage
- Non-intrusive: Read-only operations, no system modifications
- Production-ready: 24/7 datacenter operation

## Supported Platforms

| OS Family | Supported Versions | Architecture |
|-----------|--------------------|--------------|
| Ubuntu | 22.04, 24.04 | x86_64, ARM64 |
| RHEL | 8, 9, 10 | x86_64, ARM64 |
| Rocky Linux | 8, 9, 10 | x86_64, ARM64 |
| AlmaLinux | 8, 9, 10 | x86_64, ARM64 |
| Amazon Linux | 2023 | x86_64, ARM64 |

## Documentation

- [Helm Installation](docs/install-helm.md) - Kubernetes (Helm) installation and troubleshooting
- [DEB Installation](docs/install-deb.md) - Ubuntu package install, update, and uninstall
- [RPM Installation](docs/install-rpm.md) - RHEL/Rocky/Alma/Amazon package install, update, and uninstall
- [Architecture](docs/architecture.md) - Bare metal and Kubernetes architecture, dependencies, and runtime flow
- [Usage](docs/usage.md) - Commands, HTTP API, integration, and troubleshooting
- [Configuration](docs/configuration.md) - Environment variables and service configuration
- [Development](docs/development.md) - Building from source and contributing

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

Related: [leptonai/gpud](https://github.com/leptonai/gpud) (upstream dependency)

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.
