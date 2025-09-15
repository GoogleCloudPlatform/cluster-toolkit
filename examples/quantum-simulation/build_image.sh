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
