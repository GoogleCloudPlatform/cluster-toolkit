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
FROM nvcr.io/nvidia/nemo:${NEMOFW_VERSION}

ENV USE_TCPX=yes
ENV NCCL_NET=GPUDirectTCPX_v7
ENV NCCL_SOCKET_IFNAME=enp0s12
ENV NCCL_GPUDIRECTTCPX_CTRL_DEV=enp0s12
ENV NCCL_GPUDIRECTTCPX_SOCKET_IFNAME=enp6s0,enp12s0,enp134s0,enp140s0
ENV NCCL_CROSS_NIC=0
ENV NCCL_ALGO=Ring
ENV NCCL_PROTO=Simple
ENV NCCL_NSOCKS_PERTHREAD=4
ENV NCCL_SOCKET_NTHREADS=1
ENV NCCL_DYNAMIC_CHUNK_SIZE=524288
ENV NCCL_P2P_NET_CHUNKSIZE=524288
ENV NCCL_P2P_PCI_CHUNKSIZE=524288
ENV NCCL_P2P_NVL_CHUNKSIZE=1048576
ENV NCCL_BUFFSIZE=4194304
ENV CUDA_VISIBLE_DEVICES=0,1,2,3,4,5,6,7
ENV NCCL_NET_GDR_LEVEL=PIX
ENV NCCL_P2P_PXN_LEVEL=0
ENV NCCL_GPUDIRECTTCPX_UNIX_CLIENT_PREFIX=/run/tcpx
ENV NCCL_GPUDIRECTTCPX_PROGRAM_FLOW_STEERING_WAIT_MICROS=500000
ENV NCCL_GPUDIRECTTCPX_TX_BINDINGS="enp6s0:8-21,112-125;enp12s0:8-21,112-125;enp134s0:60-73,164-177;enp140s0:60-73,164-177"
ENV NCCL_GPUDIRECTTCPX_RX_BINDINGS="enp6s0:22-35,126-139;enp12s0:22-35,126-139;enp134s0:74-87,178-191;enp140s0:74-87,178-191"


RUN echo "/var/lib/tcpx/lib64" >> /etc/ld.so.conf.d/tcpx.conf && ldconfig
ENV LD_LIBRARY_PATH=/var/lib/tcpx/lib64:$LD_LIBRARY_PATH
