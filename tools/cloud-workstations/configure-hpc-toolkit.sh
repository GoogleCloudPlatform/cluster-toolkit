#!/bin/bash
# Copyright 2023 Google LLC
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

set -e

# Check if the hpc-toolkit folder exists in the user home directory
if [ ! -d "$HOME/hpc-toolkit" ]; then
	# If not, clone the repository
	git clone https://github.com/GoogleCloudPlatform/hpc-toolkit.git "$HOME/hpc-toolkit"
	cd "$HOME/hpc-toolkit" || exit
	make install-dev-deps
	pre-commit install
fi
