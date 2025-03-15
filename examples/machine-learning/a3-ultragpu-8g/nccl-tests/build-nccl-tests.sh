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

#SBATCH --exclusive
#SBATCH --ntasks=1
#SBATCH --partition=a3ultra
#SBATCH --ntasks-per-node=1
#SBATCH --gpus-per-node=8

# Usage: sbatch build-nccl-tests.sh

set -x

CONTAINER_IMAGE=./nvidia+pytorch+24.09-py3.sqsh

# Import the pytorch container to enroot if not already present.
if [ ! -f ${CONTAINER_IMAGE} ]; then
	# This creates a file named "nvidia+pytorch+24.09-py3.sqsh", which
	# uses ~18 GB of disk space. This should be run on a filesystem that
	# can be seen by all worker nodes
	enroot import docker://nvcr.io#nvidia/pytorch:24.09-py3
fi

# Install nccl-tests using openmpi from within pytorch container
srun --container-mounts="$PWD:/nccl" \
	--container-image=${CONTAINER_IMAGE} \
	bash -c "
       cd /nccl &&
       git clone https://github.com/NVIDIA/nccl-tests.git &&
       cd /nccl/nccl-tests/ &&
       MPI=1 CC=mpicc CXX=mpicxx make -j
    "
