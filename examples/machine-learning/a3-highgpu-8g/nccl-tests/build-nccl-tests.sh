#!/bin/bash
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

#SBATCH --ntasks=1
#SBATCH --partition=a3
#SBATCH --ntasks-per-node=1
#SBATCH --gpus-per-node=8

# Usage: sbatch build-nccl-tests.sh

set -x

CONTAINER_IMAGE=./nvidia+pytorch+23.10-py3.sqsh

# Install nccl-tests using openmpi from within pytorch container
srun --container-mounts="$PWD:/nccl,/var/tmp:/var/tmp" \
	--container-image=${CONTAINER_IMAGE} \
	--container-name="nccl" \
	bash -c "
     export LD_LIBRARY_PATH=/var/lib/tcpx/lib64:$LD_LIBRARY_PATH &&
       cd /nccl &&
       git clone https://github.com/NVIDIA/nccl-tests.git &&
       cd /nccl/nccl-tests/ &&
       MPI=1 CC=mpicc CXX=mpicxx make -j
    "
