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

OS_ID="$(awk -F '=' '/^ID=/ {print $2}' /etc/os-release | sed -e 's/"//g')"
OS_VERSION="$(awk -F '=' '/VERSION_ID/ {print $2}' /etc/os-release | sed -e 's/"//g')"
OS_VERSION_MAJOR="$(awk -F '=' '/VERSION_ID/ {print $2}' /etc/os-release | sed -e 's/"//g' -e 's/\..*$//')"

if { [ "${OS_ID}" = "rocky" ] || [ "${OS_ID}" = "rhel" ]; } && { [ "${OS_VERSION_MAJOR}" = "8" ]; }; then
	# Downloading the RDMA packages and installing them
	sudo dnf install https://depot.ciq.com/public/files/gce-accelerator/irdma-kernel-modules-el8-x86_64/irdma-repos.rpm -y
	sudo dnf update -y
	sudo dnf install kmod-idpf-irdma rdma-core libibverbs-utils librdmacm-utils infiniband-diags perftest -y

	# Restarting drivers so they can use RDMA without needing a reboot
	sudo rmmod idpf
	sudo rmmod irdma
	sudo modprobe idpf

	exit 0
else
	echo "Unsupported operating system ${OS_ID} ${OS_VERSION}. Cloud RDMA Drivers are only supported on Rocky Linux 8."
	exit 1
fi
