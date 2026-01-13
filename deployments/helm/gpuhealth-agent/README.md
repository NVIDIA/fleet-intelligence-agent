# gpuhealth-agent Helm chart

This chart deploys the `gpuhealth-agent` DaemonSet.

## Install

```bash
helm install gpuhealth-agent deployments/helm/gpuhealth-agent
```

## Configuration

Key values (see `values.yaml` for the full list):

- `image.repository`, `image.tag`, `image.pullPolicy`
- `resources` (CPU/memory requests and limits)
- `nodeSelector`, `tolerations`, `affinity`
- `logLevel`, `listenAddress`, `components`
- `env.DCGM_ADDRESS` (DCGM HostEngine service)

### Enrollment (per node via init container)

Enable enrollment and provide the endpoint and token:

```yaml
enroll:
  enabled: true
  endpoint: "https://api.example.com"
  tokenSecretName: "gpuhealth-token"
  tokenSecretKey: "token"
```

Create the token Secret in the same namespace:

```bash
kubectl create secret generic gpuhealth-token \
  --namespace <ns> \
  --from-literal=token='<your-enroll-token>'
```

If you prefer not to use a Secret, you can set:

```yaml
enroll:
  enabled: true
  endpoint: "https://api.example.com"
  tokenValue: "<your-enroll-token>"
```

## Notes

- The chart assumes DCGM HostEngine is already running in the cluster (typically
  via NVIDIA GPU Operator). Set `env.DCGM_ADDRESS` to match your DCGM Service.
- The DaemonSet uses `runtimeClassName: nvidia` by default.

