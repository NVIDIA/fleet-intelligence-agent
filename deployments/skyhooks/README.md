# GPUHealth Skyhook package

This folder builds a **Skyhook package image** that the [Skyhook operator](https://github.com/NVIDIA/skyhook) can run to install, configure, and uninstall the GPUHealth agent on Kubernetes nodes (host OS customization).

If you’re new to Skyhook packages, NVIDIA maintains reference packages at [NVIDIA/skyhook-packages](https://github.com/NVIDIA/skyhook-packages).

## What’s in this directory

- **`Dockerfile`**: Builds a minimal image that contains `/skyhook-package/...` (the package payload).
- **`config.json`**: Skyhook package definition. It maps lifecycle stages to scripts and marks them `on_host: true` so they run on the node.
- **`skyhook_dir/`**: Lifecycle scripts:
  - `install_*`: Install GPUHealth, enroll it, enable/start `gpuhealthd`.
  - `config_*`: Apply config from a Skyhook ConfigMap into `/etc/default/gpuhealth` and restart `gpuhealthd`.
  - `uninstall_*`: Stop/disable service and remove GPUHealth.
- **`deploy.yaml`**: Example `Skyhook` Custom Resource showing how to reference the image and provide env/config.

## Required inputs

### Enrollment environment variables

`deploy.yaml` passes these to the package:

- `GPUHEALTH_VERSION`: Package version to install.
- `GPUHEALTH_ENDPOINT`: Enrollment endpoint.
- `GPUHEALTH_TOKEN`: Enrollment token (recommended via Kubernetes Secret).

### GPUHealth OS package artifact (must be included in the image)

`install_gpuhealth.sh` expects the **`.deb` or `.rpm`** file to exist inside the package at:

`$SKYHOOK_DIR/skyhook_dir/<package-file>`

Naming conventions used by the installer:

- **Debian/Ubuntu**: `gpuhealth_<GPUHEALTH_VERSION>_<amd64|arm64>.deb`
- **RPM family**: `gpuhealth-<GPUHEALTH_VERSION>-1.<x86_64|aarch64>.rpm`

Place the appropriate artifacts under `skyhook_dir/` before building the image.

## Build & publish

Build and push the package image, then update:

- `deploy.yaml`: `.spec.packages.gpuhealth.image` and `.spec.packages.gpuhealth.version`
- `config.json`: `package_version` (keep it aligned with your image tag/versioning)

Example:

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  -t nvcr.io/nvidian/gpu-health/gpuhealth-skyhook:<tag> --push .
```

### GitHub Release pipeline (NVCR publish)

When you push a version tag (`vX.Y.Z`), the GitHub Actions release workflow also builds and pushes the Skyhook package image to NVCR **using the release `.deb`/`.rpm` artifacts** produced by GoReleaser.

It requires this GitHub Actions secret:

- `NVCR_API_KEY`: NGC API key for `nvcr.io` (docker username is the literal `$oauthtoken`)

## Deploy (example)

1. Install Skyhook in your cluster (see [NVIDIA/skyhook](https://github.com/NVIDIA/skyhook)).
2. Create the enrollment token secret referenced by `deploy.yaml` (`gpuhealth-enroll-secret`, key `token`).
3. Apply:

```bash
kubectl apply -f deployments/skyhooks/deploy.yaml
```

`deploy.yaml` also demonstrates a `gpuhealth.config` ConfigMap fragment; those `KEY=VALUE` lines are upserted into `/etc/default/gpuhealth` and the `gpuhealthd` service is restarted.


