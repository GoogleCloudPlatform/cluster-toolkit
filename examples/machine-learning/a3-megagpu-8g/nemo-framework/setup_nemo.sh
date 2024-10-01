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

#SBATCH --nodes=1
#SBATCH --ntasks-per-node=1
#SBATCH --partition=a3mega
#SBATCH --exclusive

: "${NEMOFW_VERSION:=24.07}"

srun docker build --build-arg="NEMOFW_VERSION=${NEMOFW_VERSION}" -t nemofw:tcpxo-"${NEMOFW_VERSION}" .
srun rm -f nemofw+tcpxo-"${NEMOFW_VERSION}".sqsh
srun enroot import dockerd://nemofw:tcpxo-"${NEMOFW_VERSION}"

srun \
	--container-mounts="${PWD}":/workspace/mount_dir,/var/tmp:/var/tmp \
	--container-image=./nemofw+tcpxo-"${NEMOFW_VERSION}".sqsh \
	bash -c "cp -r /opt/NeMo-Framework-Launcher/requirements.txt /opt/NeMo-Framework-Launcher/launcher_scripts /opt/NeMo-Framework-Launcher/auto_configurator /workspace/mount_dir/"
