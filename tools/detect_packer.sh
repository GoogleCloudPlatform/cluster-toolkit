#!/bin/bash
# Copyright 2026 Google LLC
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

if [[ -z $(which packer) ]]; then
	echo "no"
	exit 1
fi

# On some distributions, there may be another tool named `packer` installed by default.
# https://developer.hashicorp.com/packer/tutorials/docker-get-started/get-started-install-cli#troubleshooting
# Check if the right tool is installed by greep "help page"
if ! packer -h 2>&1 | grep "build image(s) from template" >/dev/null; then
	echo "no"
	exit 1
fi

echo "yes"
