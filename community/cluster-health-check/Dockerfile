# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM --platform=${BUILDPLATFORM} golang:1.25.10 AS build
ARG TARGETARCH
ARG BUILDARCH
ARG TARGETOS

WORKDIR /build
RUN apt-get update && apt-get install -yq --no-install-recommends \
    gcc libc6-dev \
    gcc-aarch64-linux-gnu libc6-dev-arm64-cross
COPY . .

RUN CGO_ENABLED=0 \
    GOARCH=${TARGETARCH} \
    GOOS=${TARGETOS} \
    GOTOOLCHAIN=local \
    go build -mod=vendor -o dcgm-healthcheck ./cmd/dcgm-healthcheck

# -----------------------------------------------------------------------------
# Base Debian Libraries
# -----------------------------------------------------------------------------
FROM gke.gcr.io/debian-base:bookworm-v1.0.2-gke.0 AS base
RUN apt-get update && apt-get install -y --no-install-recommends infiniband-diags

# -----------------------------------------------------------------------------
# Part 1: Base Application Image (Without DCGM dependencies)
# -----------------------------------------------------------------------------
FROM gke.gcr.io/gke-distroless/libc:gke_distroless_20240307.00_p0 AS dcgm-healthcheck-base

# Required for GPU metrics
ENV NVIDIA_DRIVER_CAPABILITIES=compute,utility,compat32
ENV NVIDIA_VISIBLE_DEVICES=all

# COPY over system library files from debian-base.
# Empirically, without these files, distroless causes a panic.
# We still avoid exposing any shell capability.
COPY --from=base /lib/* /lib/
COPY --from=base /usr/lib/*-linux-gnu/libtinfo.so.6 /lib/
COPY --from=base /bin/dmesg /bin/dmesg
COPY --from=base /usr/sbin/ibstat /usr/bin/ibstat
COPY --from=base /usr/lib/*-linux-gnu/libibumad.so* /usr/lib/
COPY --from=base /usr/lib/*-linux-gnu/libibmad.so* /usr/lib/

# Bring in dcgm-healthcheck.
COPY --from=build /build/dcgm-healthcheck /bin/dcgm-healthcheck
ENTRYPOINT ["/bin/dcgm-healthcheck"]

# -----------------------------------------------------------------------------
# Part 2: NVIDIA DCGM Source
# Pulls the official NVIDIA packages to build a functional image in 1 command
# -----------------------------------------------------------------------------
FROM --platform=linux/${TARGETARCH} nvcr.io/nvidia/cuda:12.4.1-base-ubuntu22.04 AS dcgm-nvidia
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends datacenter-gpu-manager-4-cuda12

# -----------------------------------------------------------------------------
# Final Standalone Image (For local testing / standalone deployment)
# -----------------------------------------------------------------------------
FROM dcgm-healthcheck-base AS dcgm-healthcheck

# Copy DCGM binaries
COPY --from=dcgm-nvidia /usr/bin/nv-hostengine /usr/bin/nv-hostengine
COPY --from=dcgm-nvidia /usr/bin/dcgmi /usr/bin/dcgmi

# Copy DCGM libraries. Because the source path uses a wildcard (*-linux-gnu),
# Docker flattens the output into the destination directory (/usr/lib/).
COPY --from=dcgm-nvidia /usr/lib/*-linux-gnu/libdcgm.so* /usr/lib/
COPY --from=dcgm-nvidia /usr/lib/*-linux-gnu/libdcgmmodulenvswitch.so* /usr/lib/
COPY --from=dcgm-nvidia /usr/lib/*-linux-gnu/libdcgmmodulehealth.so* /usr/lib/
COPY --from=dcgm-nvidia /usr/lib/*-linux-gnu/libdcgmmodulesysmon.so* /usr/lib/
