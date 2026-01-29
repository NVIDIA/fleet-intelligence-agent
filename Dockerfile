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
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    bash \
    sudo \
    kmod \
    util-linux \
    dmidecode \
    pciutils \
  && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/gpuhealth /usr/bin/gpuhealth

EXPOSE 15133
VOLUME ["/var/lib/gpuhealth"]

ENTRYPOINT ["/usr/bin/gpuhealth"]
CMD ["run"]
