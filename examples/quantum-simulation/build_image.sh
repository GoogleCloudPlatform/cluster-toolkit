#!/bin/bash
#SBATCH --job-name=build
#SBATCH --exclusive
#SBATCH --gpus-per-node=8
#SBATCH --ntasks-per-node=1
#SBATCH --nodes=1

# Echo all the commands, for future reference
set -eux

CONTAINER_BASENAME=cuquantum-gcp
CONTAINER_VERSION=24.08

docker build -t ${CONTAINER_BASENAME}:${CONTAINER_VERSION} .
rm -f ${CONTAINER_BASENAME}+${CONTAINER_VERSION}.sqsh || true
enroot import dockerd://${CONTAINER_BASENAME}:${CONTAINER_VERSION}
