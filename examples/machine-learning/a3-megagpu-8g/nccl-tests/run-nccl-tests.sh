#!/bin/bash
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

#SBATCH --partition=a3mega
#SBATCH --mem=0
#SBATCH -N 2
#SBATCH --gpus-per-node=8
#SBATCH --ntasks-per-node=8

# Usage: sbatch run-nccl-tests.sh

set -x
# This should be set to the squashfs file that you created for your application
CONTAINER_IMAGE=./nvidia+pytorch+24.04-py3.sqsh

# Set up NCCL Environment variables
# The following two can be useful for debugging
# export NCCL_DEBUG=INFO
# export NCCL_DEBUG_SUBSYS=INIT,NET

# These parameters should not be modified
NCCL_LIB_DIR="/var/lib/tcpxo/lib64" source /var/lib/tcpxo/lib64/nccl-env-profile.sh
export NCCL_FASTRAK_CTRL_DEV=enp0s12
export NCCL_FASTRAK_IFNAME=enp6s0,enp7s0,enp13s0,enp14s0,enp134s0,enp135s0,enp141s0,enp142s0
export NCCL_SOCKET_IFNAME=enp0s12
export NCCL_FASTRAK_LLCM_DEVICE_DIRECTORY=/dev/aperture_devices

# Here we grab all the environment variables that need to be
# passed down into the container. Slurm would otherwise only pass these env vars
# to the job environment on the host.
# shellcheck disable=SC2001
HOST_VARS=$(sed 's/ \{1,\}/,/g' <<<"${!NCCL*}")

# Mount /var/tmp to allow the rest of the enroot container to be read-only, and
# mount current $PWD to /nccl to for accessing nccl-tests binary
CONTAINER_MOUNTS="/var/tmp:/var/tmp"

# Mount PWD to /nccl in the enroot container
CONTAINER_MOUNTS=${CONTAINER_MOUNTS},"$PWD:/nccl"

# Mount required directories for GPUDirect-TCPXO functionality
CONTAINER_MOUNTS=${CONTAINER_MOUNTS},"/var/lib/tcpxo/lib64/"

# Run the workload
srun -l \
	-N "${SLURM_NNODES}" \
	--mpi=pmi2 \
	--ntasks-per-node=8 \
	--container-image="${CONTAINER_IMAGE}" \
	--container-env="${HOST_VARS}" \
	--container-mounts="${CONTAINER_MOUNTS}" \
	sh -c "
  export LD_LIBRARY_PATH=/var/lib/tcpxo/lib64:/usr/lib/x86_64-linux-gnu:\$LD_LIBRARY_PATH;
  /nccl/nccl-tests/build/all_gather_perf -b 8M -e 8G -f 2 -g 1 -w 5 --iters 200;
  "
