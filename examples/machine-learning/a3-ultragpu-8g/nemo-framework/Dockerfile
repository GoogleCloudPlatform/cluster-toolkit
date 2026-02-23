# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

ARG NEMOFW_VERSION=24.12
ARG NCCL_GIB_VERSION=v1.1.0

# Stage 1: Use the nccl-plugin-gib container as an installer
FROM us-docker.pkg.dev/gce-ai-infra/gpudirect-gib/nccl-plugin-gib:${NCCL_GIB_VERSION} AS nccl-gib
RUN /scripts/container_entry.sh install --install-nccl

# Stage 2: Build the final NeMo container
FROM nvcr.io/nvidia/nemo:$NEMOFW_VERSION

# Copy the NCCL gIB plugin and other binaries from the installer stage.
# This replaces the need to manually install and mount /var/lib/gib from the host.
COPY --from=nccl-gib /var/lib/gib /usr/local/gib

# Set the following to match what is in /usr/local/gib/scripts/set_nccl_env.sh
ENV NCCL_NET=gIB
ENV NCCL_CROSS_NIC=0
ENV NCCL_NET_GDR_LEVEL=PIX
ENV NCCL_P2P_NET_CHUNKSIZE=131072
ENV NCCL_NVLS_CHUNKSIZE=524288
ENV NCCL_IB_ADAPTIVE_ROUTING=1
ENV NCCL_IB_QPS_PER_CONNECTION=4
ENV NCCL_IB_TC=52
ENV NCCL_IB_FIFO_TC=84
# Use A3 Ultra tuner config
ENV NCCL_TUNER_CONFIG_PATH=/usr/local/gib/configs/tuner_config_a3u.txtpb


# Add NCCL DEBUG at WARN level (useful for debugging)
ENV NCCL_DEBUG=WARN
ENV NCCL_DEBUG_SUBSYS=NET,COLL,GPU,INIT,ENV
ENV GLOO_SOCKET_IFNAME=enp0s19,enp192s20

# Set up ldconfig to load nccl-gib
RUN echo "/usr/local/gib/lib64" >> /etc/ld.so.conf.d/gib.conf && ldconfig
ENV LD_LIBRARY_PATH=/usr/local/gib/lib64:$LD_LIBRARY_PATH
