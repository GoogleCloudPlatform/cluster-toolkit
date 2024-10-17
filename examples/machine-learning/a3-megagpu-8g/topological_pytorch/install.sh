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

#filename: install.sh
#submit with `sbatch install.sh`

#SBATCH --partition=a3mega
#SBATCH --gpus-per-node=8
#SBATCH --ntasks-per-node=1
#SBATCH --nodes 1

python3 -m venv env
source env/bin/activate
pip3 install --pre torch torchvision torchaudio
