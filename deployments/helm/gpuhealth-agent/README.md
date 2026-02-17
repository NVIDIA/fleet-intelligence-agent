# gpuhealth-agent Helm chart

This chart deploys the `gpuhealth-agent` DaemonSet.

## Install

For installation steps (GHCR OCI chart, enrollment, and
`helm install`/`helm upgrade`), see `docs/install-helm.md`.

## Configuration

Common values (defaults from `values.yaml`):

| Value | Default | Description |
| --- | --- | --- |
| `image.repository` | `ghcr.io/nvidia/gpuhealth-agent` | Agent image repository. |
| `image.tag` | `""` | Image tag (empty uses chart `appVersion`). |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy. |
| `imagePullSecrets` | `[]` | Optional image pull secrets (set when pulling from private registries). |
| `hostPID` | `true` | Use host PID namespace. |
| `runtimeClassName` | `nvidia` | RuntimeClass for NVIDIA runtime. |
| `securityContext.privileged` | `true` | Run privileged. |
| `securityContext.runAsUser` | `0` | Run as root. |
| `securityContext.runAsGroup` | `0` | Run as root group. |
| `env.DCGM_URL` | `nvidia-dcgm.gpu-operator.svc:5555` | DCGM HostEngine endpoint. |
| `env.DCGM_URL_IS_UNIX_SOCKET` | `"false"` | Treat `DCGM_URL` as a unix socket path. |
| `env.GPUHEALTH_COLLECT_INTERVAL` | `"1m"` | Data collection interval (1s to 24h). |
| `env.GPUHEALTH_INCLUDE_METRICS` | `"true"` | Include metrics in export. |
| `env.GPUHEALTH_INCLUDE_EVENTS` | `"true"` | Include events in export. |
| `env.GPUHEALTH_INCLUDE_MACHINEINFO` | `"true"` | Include machine info in export. |
| `env.GPUHEALTH_INCLUDE_HEALTHCHECKS` | `"true"` | Include component health data in export. |
| `env.GPUHEALTH_METRICS_LOOKBACK` | `"1m"` | Lookback window for metrics export. |
| `env.GPUHEALTH_EVENTS_LOOKBACK` | `"1m"` | Lookback window for events export. |
| `env.GPUHEALTH_CHECK_INTERVAL` | `"1m"` | Health check frequency (1s to 24h). |
| `env.GPUHEALTH_RETRY_MAX_ATTEMPTS` | `"3"` | Max retry attempts for failed exports. |
| `env.GPUHEALTH_ATTESTATION_JITTER_ENABLED` | `"true"` | Enable/disable attestation jitter. |
| `env.GPUHEALTH_ATTESTATION_INTERVAL` | `"24h"` | Attestation interval override. |
| `env.HTTP_PROXY` | `""` | Optional HTTP proxy for outbound requests. |
| `env.HTTPS_PROXY` | `""` | Optional HTTPS proxy for outbound requests. |
| `logLevel` | `warn` | Log level. |
| `listenAddress` | `0.0.0.0:15133` | Listen address. |
| `components` | `all` | Enabled components. |
| `deployEnv` | `prod` | Deployment environment (`prod`, `canary`, `stg`, `dev`) shown in Helm install notes. |
| `enroll.enabled` | `false` | Enable enrollment init container. |
| `enroll.unenroll` | `false` | Run explicit unenroll init container (cleanup persisted enrollment metadata). |
| `enroll.endpoint` | `""` | Enrollment endpoint. |
| `enroll.tokenSecretName` | `""` | Secret name for enrollment token. |
| `enroll.tokenSecretKey` | `token` | Secret key for enrollment token. |
| `enroll.tokenValue` | `""` | Inline token value (optional). |
| `enroll.securityContext.runAsUser` | `0` | Run enrollment init as root. |
| `ports.http` | `15133` | HTTP port. |
| `resources.requests.cpu` | `100m` | CPU request. |
| `resources.requests.memory` | `256Mi` | Memory request. |
| `resources.requests.ephemeral-storage` | `256Mi` | Ephemeral storage request. |
| `resources.limits.cpu` | `500m` | CPU limit. |
| `resources.limits.memory` | `512Mi` | Memory limit. |
| `resources.limits.ephemeral-storage` | `1Gi` | Ephemeral storage limit. |
| `nodeSelector` | `{"nvidia.com/gpu.present": "true"}` | Node selector (targets GPU nodes). |
| `tolerations` | `[]` | Tolerations. |
| `affinity` | `{}` | Affinity rules. |
| `serviceAccount.create` | `true` | Create ServiceAccount. |
| `serviceAccount.name` | `""` | ServiceAccount name. |
| `serviceAccount.automountToken` | `false` | Automount service account token. |

### Enrollment (per node via init container)

`enroll.enabled` and `enroll.unenroll` are mutually exclusive; do not set both to `true`.

See `docs/install-helm.md` for the enrollment flow and secret creation steps.

## Notes

- The chart assumes DCGM HostEngine is already running in the cluster (typically
  via NVIDIA GPU Operator). Set `env.DCGM_URL` to match your DCGM Service.
- The DaemonSet uses `runtimeClassName: nvidia` by default.
- **Node Labeling**: By default, the agent only deploys to nodes with GPUs (labeled `nvidia.com/gpu.present=true`).
  This label is automatically set by the NVIDIA GPU Operator or Device Plugin.
  To deploy to all nodes regardless of labels, override with `--set nodeSelector=null`.