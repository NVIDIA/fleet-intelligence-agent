# NVIDIA GPU Health Agent

Lightweight GPU health monitoring and reporting agent for NVIDIA GPU infrastructure building on top of [leptonai/gpud](https://github.com/leptonai/gpud)

## Overview

**Prerequisites:**
- NVIDIA DCGM (Data Center GPU Manager) - automatically installed from NVIDIA CUDA repositories
- See [Installation Guide](docs/installation.md) for CUDA repository setup instructions

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

## Documentation

- [Installation](docs/installation.md) - Installation, updating, and uninstalling
- [Usage](docs/usage.md) - Commands, HTTP API, integration, and troubleshooting
- [Configuration](docs/configuration.md) - Environment variables and service configuration
- [Development](docs/development.md) - Building from source and contributing

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

Related: [leptonai/gpud](https://github.com/leptonai/gpud) (upstream dependency)

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.
