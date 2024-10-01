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

ARG NEMOFW_VERSION=24.07
FROM nvcr.io/nvidia/nemo:$NEMOFW_VERSION

ENV NCCL_FASTRAK_CTRL_DEV=enp0s12
ENV NCCL_FASTRAK_IFNAME=enp6s0,enp7s0,enp13s0,enp14s0,enp134s0,enp135s0,enp141s0,enp142s0
ENV NCCL_SOCKET_IFNAME=enp0s12
ENV GLOO_SOCKET_IFNAME=enp0s12
ENV NCCL_CROSS_NIC=0
ENV NCCL_ALGO=Ring,Tree
ENV NCCL_PROTO=Simple
ENV NCCL_MIN_NCHANNELS=4
ENV NCCL_DYNAMIC_CHUNK_SIZE=524288
ENV NCCL_P2P_NET_CHUNKSIZE=524288
ENV NCCL_P2P_PCI_CHUNKSIZE=524288
ENV NCCL_P2P_NVL_CHUNKSIZE=1048576
ENV NCCL_FASTRAK_NUM_FLOWS=2
ENV NCCL_FASTRAK_USE_SNAP=1
ENV NCCL_FASTRAK_ENABLE_CONTROL_CHANNEL=0
ENV NCCL_BUFFSIZE=8388608
ENV CUDA_VISIBLE_DEVICES=0,1,2,3,4,5,6,7
ENV NCCL_NET_GDR_LEVEL=PIX
ENV NCCL_FASTRAK_ENABLE_HOTPATH_LOGGING=0
ENV NCCL_FASTRAK_USE_LLCM=1
ENV NCCL_FASTRAK_LLCM_DEVICE_DIRECTORY=/dev/aperture_devices
ENV NCCL_TUNER_PLUGIN=libnccl-tuner.io
ENV NCCL_TUNER_CONFIG_PATH=/var/lib/tcpxo/lib64/a3plus_tuner_config.textproto
ENV NCCL_SHIMNET_GUEST_CONFIG_CHECKER_CONFIG_FILE=/var/lib/tcpxo/lib64/a3plus_guest_config.textproto
ENV NCCL_FASTRAK_PLUGIN_ACCEPT_TIMEOUT_MS=600000
ENV NCCL_NVLS_ENABLE=0

RUN echo "/var/lib/tcpxo/lib64" >> /etc/ld.so.conf.d/tcpxo.conf && ldconfig
ENV LD_LIBRARY_PATH=/var/lib/tcpxo/lib64:$LD_LIBRARY_PATH
