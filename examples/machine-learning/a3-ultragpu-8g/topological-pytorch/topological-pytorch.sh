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

# shellcheck disable=SC2016
# shellcheck disable=SC2155

#filename: topological_pytorch.sh
#submit with `sbatch topological_pytorch.sh`
#SBATCH --partition=a3ultra
#SBATCH --gpus-per-node=8
#SBATCH --ntasks-per-node=8
#SBATCH --nodes 8

NCCL_LIB_DIR="/usr/local/gib/scripts" source /usr/local/gib/scripts/set_nccl_env.sh
export NCCL_SOCKET_IFNAME=enp0s19,enp192s20

source env/bin/activate

export MASTER_PORT=12345
export OMP_NUM_THREADS=12

# Demonstrate standard behavior
echo "Standard"
# Set the MASTER_ADDR to the first node in the Slurm Job Nodelist
export MASTER_ADDR=$(scontrol show hostnames "${SLURM_JOB_NODELIST}" | head -n 1)
# For torchrun, we only launch 1 task per node, and instruct torchrun to create
# 8 (SLURM_GPUS_PER_NODE) processes per node.
srun --ntasks-per-node=1 --nodes "${SLURM_NNODES}" \
	python -m torch.distributed.run \
	--nproc_per_node "${SLURM_GPUS_PER_NODE}" \
	--rdzv_endpoint "${MASTER_ADDR}":"${MASTER_PORT}" \
	--rdzv_backend c10d \
	--nnodes "${SLURM_NNODES}" topological_pytorch.py

# Demonstrate how to incorporate topology
echo "Topologically aware"
# Run 8 tasks per node (inherited from the job script), since we aren't using
# torchrun in this case. Supply the --topology flag to the script to set
# global rank and world size of variables based on Slurm
srun python topological_pytorch.py --topology
