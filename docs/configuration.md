# Configuration

This page is a quick reference for runtime configuration shared by:
- bare metal installs (`fleetintd` via systemd)
- Kubernetes installs (Helm chart)

## Where to configure

- **Bare metal**: edit `/etc/default/fleetint`, then restart:
  ```bash
  sudo systemctl restart fleetintd
  ```
- **Helm**: set values in `values.yaml` or with `--set`.
  - Env vars live under `env.*`
  - Main run flags map to chart values like `logLevel`, `listenAddress`, and `components`

## Configurable environment variables

| Environment variable | Description | Default |
|---|---|---|
| `FLEETINT_COLLECT_INTERVAL` | Data collection interval (1s to 24h) | `1m` |
| `FLEETINT_INCLUDE_METRICS` | Include metrics in export | `true` |
| `FLEETINT_INCLUDE_EVENTS` | Include events in export | `true` |
| `FLEETINT_INCLUDE_MACHINEINFO` | Include machine info in export | `true` |
| `FLEETINT_INCLUDE_HEALTHCHECKS` | Include component health data in export | `true` |
| `FLEETINT_METRICS_LOOKBACK` | Metrics lookback window | `1m` |
| `FLEETINT_EVENTS_LOOKBACK` | Events lookback window | `1m` |
| `FLEETINT_CHECK_INTERVAL` | Component health check interval (1s to 24h) | `1m` |
| `FLEETINT_RETRY_MAX_ATTEMPTS` | Max retry attempts for failed exports | `3` |
| `FLEETINT_ATTESTATION_JITTER_ENABLED` | Enable/disable attestation startup jitter | `true` |
| `FLEETINT_ATTESTATION_INTERVAL` | Attestation interval override | `24h` |
| `HTTP_PROXY` | HTTP proxy URL | empty |
| `HTTPS_PROXY` | HTTPS proxy URL | empty |

## Configurable CLI flags

These are `fleetint run` flags.

- **Bare metal**: set via `FLEETINT_FLAGS="..."` in `/etc/default/fleetint`
- **Helm**: use dedicated chart values when available

| CLI flag | Description | Default | Helm value |
|---|---|---|---|
| `--log-level` | Log level (`debug`, `info`, `warn`, `error`) | `warn` | `logLevel` |
| `--listen-address` | API bind address | bare metal: `127.0.0.1:15133` | `listenAddress` (chart default `0.0.0.0:15133`) |
| `--components` | Comma-separated enabled components (`all` or explicit list; use `all,-name` to exclude components) | `all` | `components` |
| `--enable-dcgm-policy` | Enable DCGM non-XID policy monitoring | `false` | not exposed directly |
| `--enable-fault-injection` | Enable local fault-injection endpoint (testing only) | `false` | not exposed directly |

## Component selection

Use `--components` to control which monitoring components are enabled.

Examples:

```bash
# Enable all components
fleetint run --components=all

# Enable only specific components
fleetint run --components=accelerator-nvidia-dcgm-thermal,accelerator-nvidia-dcgm-utilization,cpu,memory

# Start from the default set and disable one component
fleetint run --components=all,-accelerator-nvidia-dcgm-prof
```

Notes:

- `all` (or `*`) enables the default component set.
- Use `all,-<component-name>` to start from the default set and exclude specific components.
- A plain explicit list enables only the listed components.

Available component names:

**NVIDIA GPU components**

- `accelerator-nvidia-infiniband`
- `accelerator-nvidia-nccl`
- `accelerator-nvidia-peermem`
- `accelerator-nvidia-persistence-mode`
- `accelerator-nvidia-processes`
- `accelerator-nvidia-error-sxid`
- `accelerator-nvidia-error-xid`

**NVIDIA GPU DCGM components**

- `accelerator-nvidia-dcgm-clock`
- `accelerator-nvidia-dcgm-cpu`
- `accelerator-nvidia-dcgm-inforom`
- `accelerator-nvidia-dcgm-mem`
- `accelerator-nvidia-dcgm-nvlink`
- `accelerator-nvidia-dcgm-nvswitch`
- `accelerator-nvidia-dcgm-pcie`
- `accelerator-nvidia-dcgm-power`
- `accelerator-nvidia-dcgm-prof`
- `accelerator-nvidia-dcgm-thermal`
- `accelerator-nvidia-dcgm-utilization`

**System components**

- `cpu`
- `disk`
- `memory`
- `network-ethernet`
- `os`
- `library`

## Verify effective config

- **Bare metal**:
  ```bash
  sudo fleetint metadata
  sudo journalctl -u fleetintd -f
  ```
- **Helm**:
  ```bash
  helm get values fleet-intelligence-agent -n <namespace>
  kubectl logs -n <namespace> -l app.kubernetes.io/name=fleet-intelligence-agent --tail=100
  ```
