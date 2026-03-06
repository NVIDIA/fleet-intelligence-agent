# NVIDIA Fleet Intelligence Agent

NVIDIA Fleet Intelligence Agent - Host agent for GPU telemetry collection and attestation.

Built on top of [leptonai/gpud](https://github.com/leptonai/gpud)

## Overview

For installation prerequisites and setup details, see:
[Helm Installation](docs/install-helm.md), [DEB Installation](docs/install-deb.md), and [RPM Installation](docs/install-rpm.md).

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

| OS Family | Supported Versions | Architecture | GPU |
|-----------|--------------------|--------------|-----|
| Ubuntu | 22.04, 24.04 | x86_64, ARM64 | Hopper, Blackwell, Rubin |
| RHEL | 8, 9, 10 | x86_64, ARM64 | Hopper, Blackwell, Rubin |
| Rocky Linux | 8, 9, 10 | x86_64, ARM64 | Hopper, Blackwell, Rubin |
| AlmaLinux | 8, 9, 10 | x86_64, ARM64 | Hopper, Blackwell, Rubin |
| Amazon Linux | 2023 | x86_64, ARM64 | Hopper, Blackwell, Rubin |

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
