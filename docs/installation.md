# Installation

## Prerequisites: NVIDIA CUDA Repository

The GPU Health Agent requires NVIDIA DCGM (Data Center GPU Manager) for GPU monitoring. DCGM is available through NVIDIA's CUDA repository and will be automatically installed when you install the GPU Health Agent package.

**Before installing the GPU Health Agent**, add the appropriate NVIDIA CUDA repository for your system:

### Ubuntu/Debian Systems

```bash
# Ubuntu 22.04 (x86_64)
wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/x86_64/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb
sudo apt-get update

# Ubuntu 24.04 (x86_64)
wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2404/x86_64/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb
sudo apt-get update

# Ubuntu 22.04 (ARM64)
wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/sbsa/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb
sudo apt-get update

# Ubuntu 24.04 (ARM64)
wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2404/sbsa/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb
sudo apt-get update
```

### RHEL/Rocky/AlmaLinux Systems

```bash
# RHEL/Rocky/AlmaLinux 8 (x86_64)
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/rhel8/x86_64/cuda-rhel8.repo
sudo dnf clean all

# RHEL/Rocky/AlmaLinux 9 (x86_64)
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/rhel9/x86_64/cuda-rhel9.repo
sudo dnf clean all

# RHEL/Rocky/AlmaLinux 10 (x86_64)
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/rhel10/x86_64/cuda-rhel10.repo
sudo dnf clean all

# RHEL/Rocky/AlmaLinux 8 (ARM64)
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/rhel8/sbsa/cuda-rhel8.repo
sudo dnf clean all

# RHEL/Rocky/AlmaLinux 9 (ARM64)
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/rhel9/sbsa/cuda-rhel9.repo
sudo dnf clean all

# RHEL/Rocky/AlmaLinux 10 (ARM64)
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/rhel10/sbsa/cuda-rhel10.repo
sudo dnf clean all
```

### Amazon Linux 2023

```bash
# Amazon Linux 2023 (x86_64)
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/amzn2023/x86_64/cuda-amzn2023.repo
sudo dnf clean all

# Amazon Linux 2023 (ARM64)
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/amzn2023/sbsa/cuda-amzn2023.repo
sudo dnf clean all
```

**Note**: After adding the CUDA repository, DCGM (`datacenter-gpu-manager-4-core`) will be automatically installed as a recommended dependency when you install the GPU Health Agent package.

## Package Installation (Recommended)

Download the appropriate package from [Releases](https://github.com/NVIDIA/gpuhealth/releases):

```bash
# Ubuntu (x86_64)
sudo apt install ./gpuhealth_VERSION_amd64.deb

# Ubuntu (ARM64)
sudo apt install ./gpuhealth_VERSION_arm64.deb

# RHEL/Rocky/AlmaLinux/Amazon Linux (x86_64)
sudo dnf install ./gpuhealth-VERSION-1.x86_64.rpm

# RHEL/Rocky/AlmaLinux/Amazon Linux (ARM64)
sudo dnf install ./gpuhealth-VERSION-1.aarch64.rpm

# Verify
gpuhealth --version
systemctl status gpuhealthd
```

## Binary Installation

```bash
# Download and extract binary
tar -xzf gpuhealth_vVERSION_linux_ARCH.tar.gz
sudo cp gpuhealth /usr/local/bin/
gpuhealth --version
```

## Kubernetes Installation (Helm)

### Prerequisites

- NVIDIA GPU Operator installed with DCGM HostEngine enabled.
- A DCGM service endpoint reachable from the cluster (defaults to `nvidia-dcgm.gpu-operator.svc:5555`).
- Access to container images and Helm charts: Contact the GPU Health team to obtain an NGC API key for pulling Helm charts and container images.

Set shared variables once for the examples below:

```bash
# Namespace (override if needed)
NS=gpuhealth

# NGC API key for pulling Helm charts and container images
# Contact the GPU Health team to obtain this key
NGC_API_KEY='<ngc-api-key>'

# DCGM endpoint (usually the default is correct)
DCGM_URL='nvidia-dcgm.gpu-operator.svc:5555'

# Enrollment configuration - Go to the GPU Health UI to:
#   1. Generate an enrollment token (ENROLL_TOKEN)
#   2. Get the enrollment endpoint URL (ENROLL_ENDPOINT)
ENROLL_ENDPOINT='<enroll-endpoint>'
ENROLL_TOKEN='<enroll-token>'
ENROLL_TOKEN_SECRET_NAME='gpuhealth-enroll-token'  # Recommended secret name
```

### Add the Helm repo

```bash
helm repo add gpuhealth https://helm.ngc.nvidia.com/nvidian/gpu-health \
  --username='$oauthtoken' \
  --password="$NGC_API_KEY"
helm repo update
```

### Create NVCR image pull secret

Create the namespace first:

```bash
kubectl create namespace "$NS"
```

Create a registry secret so the DaemonSet can pull the agent image:

```bash
kubectl create secret docker-registry nvcr-pull-secret \
  --namespace "$NS" \
  --docker-server=nvcr.io \
  --docker-username='$oauthtoken' \
  --docker-password="$NGC_API_KEY"
```

### Create enrollment secret

If you need to enroll nodes, create the token Secret. The secret name should match the `ENROLL_TOKEN_SECRET_NAME` variable set above:

```bash
kubectl create secret generic "$ENROLL_TOKEN_SECRET_NAME" \
  --namespace "$NS" \
  --from-literal=token="$ENROLL_TOKEN"
```

### Install or upgrade

Install:

```bash
helm install gpuhealth-agent gpuhealth/gpuhealth-agent \
  --namespace "$NS" \
  --set enroll.enabled=true \
  --set enroll.endpoint="$ENROLL_ENDPOINT" \
  --set enroll.tokenSecretName="$ENROLL_TOKEN_SECRET_NAME"
```

Install (no enrollment):

```bash
helm install gpuhealth-agent gpuhealth/gpuhealth-agent \
  --namespace "$NS"
```

Upgrade:

```bash
helm upgrade gpuhealth-agent gpuhealth/gpuhealth-agent \
  --namespace "$NS" \
  --set enroll.enabled=true \
  --set enroll.endpoint="$ENROLL_ENDPOINT" \
  --set enroll.tokenSecretName="$ENROLL_TOKEN_SECRET_NAME"
```

Upgrade (no enrollment):

```bash
helm upgrade gpuhealth-agent gpuhealth/gpuhealth-agent \
  --namespace "$NS"
```

If DCGM is exposed at a different service name or port, set `env.DCGM_URL`:

```bash
--set env.DCGM_URL="$DCGM_URL"
```

### Verifying deployment

After installation, verify the agent is running correctly:

```bash
# Check DaemonSet status
kubectl get daemonset gpuhealth-agent -n "$NS"

# Check pods (should be one per GPU node)
kubectl get pods -n "$NS" -l app.kubernetes.io/name=gpuhealth-agent

# View pod logs
kubectl logs -n "$NS" -l app.kubernetes.io/name=gpuhealth-agent --tail=50

# Watch rollout status
kubectl rollout status daemonset/gpuhealth-agent -n "$NS"
```

Check a specific pod in detail:

```bash
# Get a pod name
POD_NAME=$(kubectl get pods -n "$NS" -l app.kubernetes.io/name=gpuhealth-agent -o jsonpath='{.items[0].metadata.name}')

# Describe the pod
kubectl describe pod -n "$NS" "$POD_NAME"

# View full logs
kubectl logs -n "$NS" "$POD_NAME" --follow
```

### Troubleshooting

**Pods not starting:**

```bash
# Check pod events
kubectl describe pod -n "$NS" -l app.kubernetes.io/name=gpuhealth-agent
```

Common issues:
- **ImagePullBackOff**: Verify the `nvcr-pull-secret` exists in the namespace
- **Pending**: Check node labels match `nodeSelector` (default: `nvidia.com/gpu.present=true`)
- **CrashLoopBackOff**: Check logs for errors

**Enrollment failures:**

```bash
# Check init container logs
kubectl logs -n "$NS" "$POD_NAME" -c enroll

# Verify enrollment secret exists
kubectl get secret "$ENROLL_TOKEN_SECRET_NAME" -n "$NS"

# Check secret content (verify token is not empty)
kubectl get secret "$ENROLL_TOKEN_SECRET_NAME" -n "$NS" -o jsonpath='{.data.token}' | base64 -d | wc -c
```

**DCGM connection issues:**

```bash
# Verify DCGM service is accessible
kubectl get svc -n gpu-operator nvidia-dcgm

# Test DCGM connectivity from a pod
kubectl exec -n "$NS" "$POD_NAME" -- curl -v telnet://nvidia-dcgm.gpu-operator.svc:5555

# Check DCGM URL environment variable
kubectl get pods -n "$NS" "$POD_NAME" -o jsonpath='{.spec.containers[0].env[?(@.name=="DCGM_URL")].value}'
```

If DCGM is at a different location, update the URL:

```bash
helm upgrade gpuhealth-agent gpuhealth/gpuhealth-agent \
  --namespace "$NS" \
  --reuse-values \
  --set env.DCGM_URL="<dcgm-service>:<port>"
```

### Node Scheduling

**By default**, the agent automatically deploys only to GPU nodes using the nodeSelector:

```yaml
nodeSelector:
  nvidia.com/gpu.present: "true"
```

This label is automatically set by the NVIDIA GPU Operator or Device Plugin, so no manual node labeling is required.

If you need a different node selector or tolerations for GPU taints, you can override them.

Using `--set` (quote the tolerations for zsh, and escape dots in the label key):

```bash
helm upgrade --install gpuhealth-agent gpuhealth/gpuhealth-agent \
  --namespace "$NS" \
  --set-string nodeSelector.nvidia\\.com/gpu\\.deploy\\.dcgm=true \
  --set 'tolerations[0].key=nvidia.com/gpu' \
  --set 'tolerations[0].operator=Exists' \
  --set 'tolerations[0].effect=NoSchedule'
```

Using a values file:

```yaml
nodeSelector:
  nvidia.com/gpu.deploy.dcgm: "true"

tolerations:
  - key: "nvidia.com/gpu"
    operator: "Exists"
    effect: "NoSchedule"
```

## Build from Source

```bash
git clone https://github.com/NVIDIA/gpuhealth.git
cd gpuhealth
make gpuhealth
sudo cp bin/gpuhealth /usr/local/bin/
```

## System Requirements

### Supported Operating Systems

| OS Family | Supported Versions | Architecture |
|-----------|-------------------|--------------|
| Ubuntu | 22.04, 24.04 | x86_64, ARM64 |
| RHEL | 8, 9, 10 | x86_64, ARM64 |
| Rocky Linux | 8, 9, 10 | x86_64, ARM64 |
| AlmaLinux | 8, 9, 10 | x86_64, ARM64 |
| Amazon Linux | 2023 | x86_64, ARM64 |

**Note**: The agent may run on other Linux distributions, but only the distributions listed above are officially supported and tested. Compatibility with other distributions is not guaranteed.

### Supported GPU Architectures

| Architecture | Type | Examples |
|--------------|------|----------|
| GPU | Hopper | H100, H200, GH200 |
| GPU | Blackwell | GB200, GB300, B200, B300 |

**NVIDIA Driver**: 535+

**Note**: Other GPU architectures and earlier driver versions may work but are not guaranteed. The agent uses NVML (NVIDIA Management Library) which is included with the NVIDIA driver. System monitoring features work without GPUs.

### Resource Requirements

- **Memory**: <100MB RAM
- **CPU**: <1% CPU usage
- **Network**: Optional, for remote export features

## Updating

Simply install the new package version:

```bash
# Ubuntu (x86_64)
sudo apt install ./gpuhealth_VERSION_amd64.deb

# Ubuntu (ARM64)
sudo apt install ./gpuhealth_VERSION_arm64.deb

# RHEL/Rocky/AlmaLinux/Amazon Linux (x86_64)
sudo dnf install ./gpuhealth-VERSION-1.x86_64.rpm

# RHEL/Rocky/AlmaLinux/Amazon Linux (ARM64)
sudo dnf install ./gpuhealth-VERSION-1.aarch64.rpm
```

The service will automatically restart with the new version.

## Uninstalling

```bash
# Ubuntu
sudo apt remove gpuhealth
sudo apt purge gpuhealth  # Also removes configuration files

# RHEL/Rocky/AlmaLinux
sudo dnf remove gpuhealth
```
