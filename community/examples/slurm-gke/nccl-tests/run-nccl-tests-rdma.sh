#!/bin/bash
# Copyright 2025 Google LLC
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

#SBATCH --partition=gke
#SBATCH --mem=0
#SBATCH -N 2
#SBATCH --gpus-per-node=8
#SBATCH --ntasks-per-node=8
#SBATCH --exclusive

# Usage: sbatch run-nccl-tests-rdma.sh

set -x
# This should be set to the squashfs file that you created for your application
CONTAINER_IMAGE=./nvidia+pytorch+24.09-py3.sqsh

# Set up NCCL Environment variables
# The following two can be useful for debugging
# export NCCL_DEBUG=INFO
# export NCCL_DEBUG_SUBSYS=INIT,NET

# These parameters should not be modified
source /usr/local/gib/scripts/set_nccl_env.sh
export NCCL_NET=gIB

# Mount /var/tmp to allow the rest of the enroot container to be read-only, and
# mount current $PWD to /nccl to for accessing nccl-tests binary
CONTAINER_MOUNTS="/var/tmp:/var/tmp"

# Mount PWD to /nccl in the enroot container
CONTAINER_MOUNTS=${CONTAINER_MOUNTS},"$PWD:/nccl"

# Mount required directories for gIB libnccl-net
CONTAINER_MOUNTS=${CONTAINER_MOUNTS},"/usr/local/gib"

# Run the workload
srun -l \
	-N "${SLURM_NNODES}" \
	--ntasks-per-node=8 \
	--mpi=pmi2 \
	--container-image="${CONTAINER_IMAGE}" \
	--container-mounts="${CONTAINER_MOUNTS}" \
	sh -c "export LD_LIBRARY_PATH=/usr/local/gib/lib64:/usr/lib/x86_64-linux-gnu:\$LD_LIBRARY_PATH;
  /nccl/nccl-tests/build/all_gather_perf -b 256M -e 8G -f 2 -g 1 -w 5 --iters 200;
  "
