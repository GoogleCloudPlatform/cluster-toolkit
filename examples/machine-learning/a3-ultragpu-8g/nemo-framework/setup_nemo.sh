#!/bin/bash
# Copyright 2026 "Google LLC"
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

#SBATCH --nodes=1
#SBATCH --ntasks-per-node=1
#SBATCH --partition=a3ultra
#SBATCH --exclusive

: "${NEMOFW_VERSION:=24.12}"
: "${NCCL_GIB_VERSION:=v1.1.0}"

# This ensures that the docker process on the compute node can access us-docker.pkg.dev
srun gcloud auth configure-docker us-docker.pkg.dev --quiet

srun docker build \
	--build-arg="NEMOFW_VERSION=${NEMOFW_VERSION}" \
	--build-arg="NCCL_GIB_VERSION=${NCCL_GIB_VERSION}" \
	-t nemo-"${NEMOFW_VERSION}"-"${NCCL_GIB_VERSION}" .
srun rm -f nemo-"${NEMOFW_VERSION}"-"${NCCL_GIB_VERSION}".sqsh
srun enroot import dockerd://nemo-"${NEMOFW_VERSION}"-"${NCCL_GIB_VERSION}"

srun \
	--container-mounts="${PWD}":/workspace/mount_dir,/var/tmp:/var/tmp \
	--container-image=./nemo-"${NEMOFW_VERSION}"-"${NCCL_GIB_VERSION}".sqsh \
	bash -c "cp -r /opt/NeMo-Framework-Launcher/requirements.txt /opt/NeMo-Framework-Launcher/launcher_scripts /opt/NeMo-Framework-Launcher/auto_configurator /workspace/mount_dir/"
