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

# This creates a file named "nvidia+pytorch+23.10-py3.sqsh", which
# uses ~18 GB of disk space. This should be run on a filesystem that
# can be seen by all worker nodes

#SBATCH --exclusive
#SBATCH -N 1
#SBATCH --partition=a3ultra
#SBATCH --ntasks-per-node=1
#SBATCH --gpus-per-node=8

if [ -d /run/enroot ]; then
    echo "Enroot directory /run/enroot already exists"
else
    echo "Creating enroot directory /run/enroot"
    mkdir -p /run/enroot
    chmod 700 /run/enroot
fi

srun enroot import docker://nvcr.io#nvidia/pytorch:23.10-py3
