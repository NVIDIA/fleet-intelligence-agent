# Configuration

## Service Configuration

After package installation, edit `/etc/default/gpuhealth` to configure the service:

```bash
sudo nano /etc/default/gpuhealth
sudo systemctl restart gpuhealthd
```

**Default configuration** (`/etc/default/gpuhealth`):
```bash
GPUHEALTH_FLAGS="--log-level=warn --components=all,-docker,-kubelet,-tailscale,-containerd,-fuse,-nfs"
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

## Remote Export Configuration

To enable remote telemetry export to an OpenTelemetry-compatible endpoint:

```bash
sudo gpuhealth register --endpoint "https://telemetry.company.com/v1" --token "your-token"
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

Adjust collection and check intervals based on your monitoring needs:

```bash
# More frequent monitoring (30 seconds)
GPUHEALTH_COLLECT_INTERVAL=30s
GPUHEALTH_CHECK_INTERVAL=30s

# Less frequent monitoring (5 minutes)
GPUHEALTH_COLLECT_INTERVAL=5m
GPUHEALTH_CHECK_INTERVAL=5m
```


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

# Enable all except specific ones (default)
GPUHEALTH_FLAGS="--log-level=warn --components=all,-docker,-kubelet,-tailscale"

# Enable only specific components
GPUHEALTH_FLAGS="--log-level=warn --components=gpu,infiniband,cpu,memory"
```

**Available components:**

**NVIDIA GPU Components:**
- `accelerator-nvidia-bad-envs` - NVIDIA environment variables validation
- `accelerator-nvidia-clock-speed` - GPU clock speeds
- `accelerator-nvidia-ecc` - ECC memory errors
- `accelerator-nvidia-fabric-manager` - NVIDIA Fabric Manager status
- `accelerator-nvidia-gpm` - GPU Performance Monitor
- `accelerator-nvidia-gsp-firmware-mode` - GSP firmware mode
- `accelerator-nvidia-gpu-counts` - GPU count validation
- `accelerator-nvidia-hw-slowdown` - Hardware slowdown detection
- `accelerator-nvidia-infiniband` - InfiniBand monitoring
- `accelerator-nvidia-memory` - GPU memory usage
- `accelerator-nvidia-nccl` - NCCL library status
- `accelerator-nvidia-nvlink` - NVLink status
- `accelerator-nvidia-peermem` - Peer memory access
- `accelerator-nvidia-persistence-mode` - Persistence mode status
- `accelerator-nvidia-power` - GPU power consumption
- `accelerator-nvidia-processes` - GPU processes
- `accelerator-nvidia-remapped-rows` - Memory remapped rows
- `accelerator-nvidia-sxid` - NVIDIA Sxid errors
- `accelerator-nvidia-temperature` - GPU temperature
- `accelerator-nvidia-utilization` - GPU utilization
- `accelerator-nvidia-xid` - NVIDIA Xid errors

**System Components:**
- `cpu` - CPU monitoring
- `disk` - Disk I/O and space monitoring
- `memory` - System memory monitoring
- `network` - Network interface monitoring
- `os` - Operating system information
- `kernel-module` - Kernel module status
- `library` - System library information
- `pci` - PCI device information

**Container/Orchestration Components (disabled by default):**
- `containerd` - Containerd monitoring
- `docker` - Docker container monitoring
- `kubelet` - Kubernetes kubelet monitoring

**Other Components (disabled by default):**
- `fuse` - FUSE filesystem monitoring
- `nfs` - NFS monitoring
- `tailscale` - Tailscale VPN monitoring

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

