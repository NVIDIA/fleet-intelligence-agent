# Configuration

This page is a quick reference for runtime configuration shared by:
- bare metal installs (`gpuhealthd` via systemd)
- Kubernetes installs (Helm chart)

## Where to configure

- **Bare metal**: edit `/etc/default/gpuhealth`, then restart:
  ```bash
  sudo systemctl restart gpuhealthd
  ```
- **Helm**: set values in `values.yaml` or with `--set`.
  - Env vars live under `env.*`
  - Main run flags map to chart values like `logLevel`, `listenAddress`, and `components`

## Configurable environment variables

| Environment variable | Description | Default |
|---|---|---|
| `GPUHEALTH_COLLECT_INTERVAL` | Data collection interval (1s to 24h) | `1m` |
| `GPUHEALTH_INCLUDE_METRICS` | Include metrics in export | `true` |
| `GPUHEALTH_INCLUDE_EVENTS` | Include events in export | `true` |
| `GPUHEALTH_INCLUDE_MACHINEINFO` | Include machine info in export | `true` |
| `GPUHEALTH_INCLUDE_HEALTHCHECKS` | Include component health data in export | `true` |
| `GPUHEALTH_METRICS_LOOKBACK` | Metrics lookback window | `1m` |
| `GPUHEALTH_EVENTS_LOOKBACK` | Events lookback window | `1m` |
| `GPUHEALTH_CHECK_INTERVAL` | Component health check interval (1s to 24h) | `1m` |
| `GPUHEALTH_RETRY_MAX_ATTEMPTS` | Max retry attempts for failed exports | `3` |
| `GPUHEALTH_ATTESTATION_JITTER_ENABLED` | Enable/disable attestation startup jitter | `true` |
| `GPUHEALTH_ATTESTATION_INTERVAL` | Attestation interval override | `24h` |
| `HTTP_PROXY` | HTTP proxy URL | empty |
| `HTTPS_PROXY` | HTTPS proxy URL | empty |

## Configurable CLI flags

These are `gpuhealth run` flags.

- **Bare metal**: set via `GPUHEALTH_FLAGS="..."` in `/etc/default/gpuhealth`
- **Helm**: use dedicated chart values when available

| CLI flag | Description | Default | Helm value |
|---|---|---|---|
| `--log-level` | Log level (`debug`, `info`, `warn`, `error`) | `warn` | `logLevel` |
| `--listen-address` | API bind address | bare metal: `127.0.0.1:15133` | `listenAddress` (chart default `0.0.0.0:15133`) |
| `--components` | Comma-separated enabled components (`all`, `none`, or explicit list) | `all` | `components` |
| `--enable-dcgm-policy` | Enable DCGM non-XID policy monitoring | `false` | not exposed directly |
| `--enable-fault-injection` | Enable local fault-injection endpoint (testing only) | `false` | not exposed directly |

## Verify effective config

- **Bare metal**:
  ```bash
  sudo gpuhealth metadata
  sudo journalctl -u gpuhealthd -f
  ```
- **Helm**:
  ```bash
  helm get values gpuhealth-agent -n <namespace>
  kubectl logs -n <namespace> -l app.kubernetes.io/name=gpuhealth-agent --tail=100
  ```
