# Configuration

## Service Configuration

After package installation, edit `/etc/default/gpuhealth` to configure the service:

```bash
sudo nano /etc/default/gpuhealth
sudo systemctl restart gpuhealthd
```

**Default configuration** (`/etc/default/gpuhealth`):
```bash
GPUHEALTH_FLAGS="--log-level=warn"
GPUHEALTH_COLLECT_INTERVAL="1m"
GPUHEALTH_INCLUDE_METRICS="true"
GPUHEALTH_INCLUDE_EVENTS="true"
GPUHEALTH_INCLUDE_MACHINEINFO="true"
GPUHEALTH_INCLUDE_HEALTHCHECKS="true"
GPUHEALTH_METRICS_LOOKBACK="1m"
GPUHEALTH_EVENTS_LOOKBACK="1m"
GPUHEALTH_CHECK_INTERVAL="1m"
GPUHEALTH_RETRY_MAX_ATTEMPTS="3"
```

By default, the agent monitors all components except docker, kubelet, tailscale, containerd, fuse, and nfs.

## Environment Variables

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `GPUHEALTH_FLAGS` | Additional command line flags | `--log-level=warn` |
| `GPUHEALTH_COLLECT_INTERVAL` | Data collection interval (1s to 24h) | `1m` |
| `GPUHEALTH_INCLUDE_METRICS` | Include metrics in export | `true` |
| `GPUHEALTH_INCLUDE_EVENTS` | Include events in export | `true` |
| `GPUHEALTH_INCLUDE_MACHINEINFO` | Include machine info | `true` |
| `GPUHEALTH_INCLUDE_HEALTHCHECKS` | Include component health data | `true` |
| `GPUHEALTH_METRICS_LOOKBACK` | How far back to look for metrics | `1m` |
| `GPUHEALTH_EVENTS_LOOKBACK` | How far back to look for events | `1m` |
| `GPUHEALTH_CHECK_INTERVAL` | Component health check frequency (1s to 24h) | `1m` |
| `GPUHEALTH_RETRY_MAX_ATTEMPTS` | Max retry attempts for failed exports | `3` |
| `HTTP_PROXY` | HTTP proxy server URL | - |
| `HTTPS_PROXY` | HTTPS proxy server URL | - |

## Remote Export Configuration

To enable remote telemetry export to an OpenTelemetry-compatible endpoint:

```bash
sudo gpuhealth enroll --endpoint "https://api.example.com" --token "your-token"
```

This configures the agent to send telemetry data via OTLP over HTTP to the specified endpoint.

**Verify Configuration:**

```bash
sudo gpuhealth metadata
```

## HTTP Proxy Configuration

For environments that require HTTP proxies:

```bash
# Add to /etc/default/gpuhealth
HTTP_PROXY="http://proxy.company.com:8080"
HTTPS_PROXY="http://proxy.company.com:8080"
```

**With Authentication:**

```bash
HTTP_PROXY="http://username:password@proxy.company.com:8080"
HTTPS_PROXY="http://username:password@proxy.company.com:8080"
```

## Data Collection Intervals

Adjust collection and check intervals based on your monitoring needs. Both intervals must be between **1 second and 24 hours**:

```bash
# More frequent monitoring (30 seconds)
GPUHEALTH_COLLECT_INTERVAL=30s
GPUHEALTH_CHECK_INTERVAL=30s

# Less frequent monitoring (5 minutes)
GPUHEALTH_COLLECT_INTERVAL=5m
GPUHEALTH_CHECK_INTERVAL=5m

# Maximum interval (daily)
GPUHEALTH_COLLECT_INTERVAL=24h
GPUHEALTH_CHECK_INTERVAL=24h
```

**Note:** Values outside this range (e.g., `25h`) will cause the agent to fail to start with an error message.


## Common Configuration Examples

### Change API Server Port

Edit `/etc/default/gpuhealth` and set the `--listen-address` flag:

```bash
GPUHEALTH_FLAGS="--log-level=warn --listen-address=0.0.0.0:8080"
```

Then restart the service:

```bash
sudo systemctl restart gpuhealthd
```

### Reduce Logging Level

Edit `/etc/default/gpuhealth` and change the log level:

```bash
GPUHEALTH_FLAGS="--log-level=error"
```

Available levels: `debug`, `info`, `warn`, `error`

### Metrics-Only Export

Control what data is included in exports:

```bash
# Metrics only (exclude events, machine info, and health checks)
GPUHEALTH_INCLUDE_METRICS=true
GPUHEALTH_INCLUDE_EVENTS=false
GPUHEALTH_INCLUDE_MACHINEINFO=false
GPUHEALTH_INCLUDE_HEALTHCHECKS=false
```

### Enable/Disable Components

The `--components` flag controls which monitoring components are enabled:

```bash
# Enable all components
GPUHEALTH_FLAGS="--log-level=warn --components=all"

# Disable all components
GPUHEALTH_FLAGS="--log-level=warn --components=none"

# Enable only specific components
GPUHEALTH_FLAGS="--log-level=warn --components=accelerator-nvidia-dcgm-thermal,accelerator-nvidia-dcgm-utilization,cpu,memory"
```

**Available components:**

**NVIDIA GPU Components:**
- `accelerator-nvidia-fabric-manager` - NVIDIA Fabric Manager status
- `accelerator-nvidia-gpu-counts` - GPU count validation
- `accelerator-nvidia-infiniband` - InfiniBand monitoring
- `accelerator-nvidia-nccl` - NCCL library status
- `accelerator-nvidia-nvlink` - NVLink status
- `accelerator-nvidia-peermem` - Peer memory access
- `accelerator-nvidia-persistence-mode` - Persistence mode status
- `accelerator-nvidia-processes` - GPU processes
- `accelerator-nvidia-error-sxid` - NVIDIA Sxid errors

**NVIDIA GPU (DCGM) Components:**
- `accelerator-nvidia-dcgm-clock` - GPU clock speeds
- `accelerator-nvidia-dcgm-cpu` - CPU-related DCGM health/telemetry
- `accelerator-nvidia-dcgm-inforom` - GPU InfoROM health/telemetry
- `accelerator-nvidia-dcgm-mem` - GPU memory health/telemetry
- `accelerator-nvidia-dcgm-nvlink` - NVLink health/telemetry (DCGM)
- `accelerator-nvidia-dcgm-nvswitch` - NVSwitch health/telemetry (DCGM)
- `accelerator-nvidia-dcgm-pcie` - PCIe health/telemetry (DCGM)
- `accelerator-nvidia-dcgm-power` - GPU power health/telemetry (DCGM)
- `accelerator-nvidia-dcgm-prof` - GPU profiling/perf metrics (DCGM)
- `accelerator-nvidia-dcgm-thermal` - GPU thermals (DCGM)
- `accelerator-nvidia-dcgm-utilization` - GPU utilization (DCGM)
- `accelerator-nvidia-dcgm-xid` - NVIDIA Xid errors (DCGM)

**System Components:**
- `cpu` - CPU monitoring
- `disk` - Disk I/O and space monitoring
- `memory` - System memory monitoring
- `network-ethernet` - Network interface monitoring (ethernet)
- `network-latency` - Network latency monitoring
- `os` - Operating system information
- `kernel-module` - Kernel module status
- `library` - System library information
- `pci` - PCI device information

## Troubleshooting Configuration

**View Current Configuration:**

```bash
sudo gpuhealth metadata
```

**Check Logs:**

```bash
sudo journalctl -u gpuhealthd -f
```

**Validate Configuration File:**

```bash
cat /etc/default/gpuhealth
```

