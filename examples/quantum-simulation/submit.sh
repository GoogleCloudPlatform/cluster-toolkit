#!/bin/bash
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
CONTAINER_MOUNTS="${PWD}:/home"
CONTAINER_BASENAME=cuquantum-gcp
CONTAINER_VERSION=24.08
CONTAINER_NAME=${CONTAINER_BASENAME}+${CONTAINER_VERSION}.sqsh


srun -l --mpi=pmix \
 --cpu-bind=verbose \
 --container-image=./${CONTAINER_NAME} \
 --container-writable \
 --container-mounts=${CONTAINER_MOUNTS} \
 --wait=10 \
 --kill-on-bad-exit=1 \
 bash -c "
 set -x
 export UCX_NET_DEVICES=mlx5_0:1,mlx5_1:1,mlx5_2:1,mlx5_3:1,mlx5_4:1,mlx5_5:1,mlx5_6:1,mlx5_7:1;
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
