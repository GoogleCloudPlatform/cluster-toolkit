#!/bin/bash
# Copyright 2025 "Google LLC"
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

#SBATCH --mem=0
#SBATCH --job-name=39-8-2
#SBATCH --exclusive
#SBATCH --gpus-per-node=8
#SBATCH --ntasks-per-node=8
#SBATCH --nodes=2
#SBATCH --time=00:30:00

# Echo all the commands, for future reference
set -x

# Mount /var/tmp to allow the rest of the enroot container to be read-only
CONTAINER_MOUNTS="${PWD}:/home/cuquantum"
CONTAINER_BASENAME=cuquantum-gcp
CONTAINER_VERSION=24.08
CONTAINER_NAME=${CONTAINER_BASENAME}+${CONTAINER_VERSION}.sqsh
UCX_NET_DEVICES=$(ibv_devinfo -l | grep -v ":" | xargs -I {} echo {}:1 | xargs echo | sed "s/ /,/g")
export UCX_NET_DEVICES

srun -l --mpi=pmix \
	--cpu-bind=verbose \
	--container-image=./${CONTAINER_NAME} \
	--container-writable \
	--container-mounts="${CONTAINER_MOUNTS}" \
	--wait=10 \
	--kill-on-bad-exit=1 \
	bash -c "
 set -x
 cd /home/cuquantum/;
 /opt/conda/envs/cuquantum-24.08/bin/cuquantum-benchmarks circuit \
    -v \
    --frontend qiskit \
    --backend cusvaer \
    --benchmark qpe \
    --precision double \
    --nfused 5 \
    --nqubits 36 \
    --cachedir data_36 \
    --cusvaer-global-index-bits 3,1 \
    --cusvaer-p2p-device-bits 3
"
