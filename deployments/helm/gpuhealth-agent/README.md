# gpuhealth-agent Helm chart

This chart deploys the `gpuhealth-agent` DaemonSet.

## Install

For installation steps (NGC Helm repo, image pull secret, enrollment, and
`helm install`/`helm upgrade`), see `docs/installation.md`.

## Configuration

Common values (defaults from `values.yaml`):

| Value | Default | Description |
| --- | --- | --- |
| `image.repository` | `nvcr.io/0753215517602916/agent-artifact/gpuhealth-agent` | Agent image repository. |
| `image.tag` | `""` | Image tag (empty uses chart `appVersion`). |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy. |
| `imagePullSecrets[0].name` | `nvcr-pull-secret` | Secret for pulling from NVCR. |
| `hostPID` | `true` | Use host PID namespace. |
| `runtimeClassName` | `nvidia` | RuntimeClass for NVIDIA runtime. |
| `securityContext.privileged` | `true` | Run privileged. |
| `securityContext.runAsUser` | `0` | Run as root. |
| `securityContext.runAsGroup` | `0` | Run as root group. |
| `env.DCGM_URL` | `nvidia-dcgm.gpu-operator.svc:5555` | DCGM HostEngine endpoint. |
| `env.DCGM_URL_IS_UNIX_SOCKET` | `"false"` | Treat `DCGM_URL` as a unix socket path. |
| `logLevel` | `warn` | Log level. |
| `listenAddress` | `0.0.0.0:15133` | Listen address. |
| `components` | `all` | Enabled components. |
| `enroll.enabled` | `false` | Enable enrollment init container. |
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

See `docs/installation.md` for the enrollment flow and secret creation steps.

## Notes

- The chart assumes DCGM HostEngine is already running in the cluster (typically
  via NVIDIA GPU Operator). Set `env.DCGM_URL` to match your DCGM Service.
- The DaemonSet uses `runtimeClassName: nvidia` by default.
- **Node Labeling**: By default, the agent only deploys to nodes with GPUs (labeled `nvidia.com/gpu.present=true`).
  This label is automatically set by the NVIDIA GPU Operator or Device Plugin.
  To deploy to all nodes regardless of labels, override with `--set nodeSelector=null`.