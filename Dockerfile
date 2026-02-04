# This repo depends on a private module on gitlab-master.nvidia.com (see go.mod replace),
# so you must provide credentials at build time via SSH agent forwarding:
#     eval "$(ssh-agent -s)"; ssh-add ~/.ssh/id_ed25519
#     DOCKER_BUILDKIT=1 docker build --ssh default -t gpuhealth:dev .
ARG DCGM_VERSION="4.4.2-1-ubuntu22.04"

FROM golang:1.24.12 AS build

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    git \
    openssh-client \
    build-essential \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /src

RUN git config --global url."ssh://git@gitlab-master.nvidia.com:12051/".insteadOf "https://gitlab-master.nvidia.com/"
RUN mkdir -p /root/.ssh && ssh-keyscan -p 12051 gitlab-master.nvidia.com >> /root/.ssh/known_hosts

ARG GOPRIVATE=gitlab-master.nvidia.com
ENV GOPRIVATE=${GOPRIVATE}
ENV GONOSUMDB=${GOPRIVATE}
# Ignore local go.work during container builds (no ../gpud in context)
ENV GOWORK=off

COPY go.mod go.sum ./

# Download modules with ephemeral credentials (not persisted in image layers).
RUN --mount=type=ssh /bin/sh -ceu 'go mod download'

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH
ARG BUILD_TIMESTAMP=""
ARG VERSION="0.0.1+container"
ARG REVISION=""

RUN CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH:-amd64} \
  go build -trimpath \
    -ldflags "-s -w \
      -X github.com/NVIDIA/gpuhealth/internal/version.BuildTimestamp=${BUILD_TIMESTAMP} \
      -X github.com/NVIDIA/gpuhealth/internal/version.Version=${VERSION} \
      -X github.com/NVIDIA/gpuhealth/internal/version.Revision=${REVISION} \
      -X github.com/NVIDIA/gpuhealth/internal/version.Package=github.com/NVIDIA/gpuhealth" \
    -o /out/gpuhealth ./cmd/gpuhealth

FROM nvcr.io/nvidia/cloud-native/dcgm:${DCGM_VERSION} AS runtime

ENV DEBIAN_FRONTEND=noninteractive
RUN set -eux; \
  apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    bash \
    sudo \
    kmod \
    util-linux \
    dmidecode \
    pciutils \
    wget \
    gnupg \
  ; \
  arch="$(dpkg --print-architecture)"; \
  case "${arch}" in \
    amd64) cuda_arch="x86_64" ;; \
    arm64) cuda_arch="sbsa" ;; \
    *) echo "unsupported arch: ${arch}" >&2; exit 1 ;; \
  esac; \
  wget -qO /usr/share/keyrings/cuda-archive-keyring.gpg \
    "https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/${cuda_arch}/cuda-archive-keyring.gpg"; \
  echo "deb [signed-by=/usr/share/keyrings/cuda-archive-keyring.gpg] \
    https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/${cuda_arch}/ /" \
    > /etc/apt/sources.list.d/cuda.list; \
  apt-get update; \
  apt-get install -y --no-install-recommends \
    nvattest \
    corelib \
  ; \
  rm -rf /var/lib/apt/lists/*

COPY --from=build /out/gpuhealth /usr/bin/gpuhealth

EXPOSE 15133
VOLUME ["/var/lib/gpuhealth"]

ENTRYPOINT ["/usr/bin/gpuhealth"]
CMD ["run"]
