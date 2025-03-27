# Copyright 2024 "Google LLC"
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

ARG NEMOFW_VERSION=24.12
FROM nvcr.io/nvidia/nemo:$NEMOFW_VERSION

ENV NCCL_DEBUG=INFO,WARN
ENV NCCL_DEBUG_SUBSYS=NET,COLL,GPU,INIT,ENV
ENV NCCL_SOCKET_IFNAME=enp0s19,enp192s20
ENV GLOO_SOCKET_IFNAME=enp0s19,enp192s20
ENV NCCL_CROSS_NIC=0
ENV NCCL_NET_GDR_LEVEL=PIX
#ENV PMIX_MCA_gds=^ds12
ENV NCCL_P2P_NET_CHUNKSIZE=131072
ENV NCCL_P2P_PCI_CHUNKSIZE=131072
ENV NCCL_P2P_NVL_CHUNKSIZE=524288
ENV NCCL_NVLS_CHUNKSIZE=524288
ENV NCCL_IB_GID_INDEX=3
ENV NCCL_IB_ADAPTIVE_ROUTING=1
ENV NCCL_IB_QPS_PER_CONNECTION=4
ENV NCCL_IB_TC=52
ENV NCCL_IB_FIFO_TC=84
ENV NCCL_SHIMNET_GUEST_CONFIG_CHECKER_CONFIG_FILE=/usr/local/gib/configs/guest_config.txtpb
ENV NCCL_TUNER_CONFIG_PATH=/usr/local/gib/configs/tuner_config.txtpb
ENV NCCL_NET=gIB

RUN echo "/usr/local/gib/lib64" >> /etc/ld.so.conf.d/gib.conf && ldconfig
ENV LD_LIBRARY_PATH=/usr/local/gib/lib64:$LD_LIBRARY_PATH
