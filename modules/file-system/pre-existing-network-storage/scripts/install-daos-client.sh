#!/bin/bash
# Copyright 2024 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e -o pipefail

OS_ID=$(awk -F '=' '/^ID=/ {print $2}' /etc/os-release | sed -e 's/"//g')
OS_VERSION=$(awk -F '=' '/VERSION_ID/ {print $2}' /etc/os-release | sed -e 's/"//g')
OS_VERSION_MAJOR=$(awk -F '=' '/VERSION_ID/ {print $2}' /etc/os-release | sed -e 's/"//g' -e 's/\..*$//')

if ! {
	{ [[ "${OS_ID}" = "rocky" ]] || [[ "${OS_ID}" = "rhel" ]]; } && { [[ "${OS_VERSION_MAJOR}" = "8" ]] || [[ "${OS_VERSION_MAJOR}" = "9" ]]; } ||
		{ [[ "${OS_ID}" = "ubuntu" ]] && [[ "${OS_VERSION}" = "22.04" ]]; } ||
		{ [[ "${OS_ID}" = "debian" ]] && [[ "${OS_VERSION_MAJOR}" = "12" ]]; }
}; then
	echo "Unsupported operating system ${OS_ID} ${OS_VERSION}. This script only supports Rocky Linux 8, Redhat 8, Redhat 9, Ubuntu 22.04, and Debian 12."
	exit 1
fi

if [ -x /bin/daos ]; then
	echo "DAOS already installed"
	daos version	
fi

exit 0
