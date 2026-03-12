# SPDX-FileCopyrightText: Copyright (c) 2025, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
# Builds the fleetint-otelcol custom collector binary.
#
# Build from the repository root:
#   docker build -f otelcol/Dockerfile -t fleetint-otelcol:latest .

FROM golang:1.24 AS builder

WORKDIR /src
# Copy the entire repo so otelcol/go.mod can reference auth/sakauth sources.
COPY . .

WORKDIR /src/otelcol
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /bin/fleetint-otelcol .

# ---

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /bin/fleetint-otelcol /fleetint-otelcol

EXPOSE 4318 13133

ENTRYPOINT ["/fleetint-otelcol"]
