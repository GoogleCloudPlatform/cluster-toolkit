#!/bin/bash
# Copyright 2026 Google LLC
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

# Install Managed Lustre client modules
# Based on these instructions: https://cloud.google.com/managed-lustre/docs/connect-from-compute-engine

# The client modules currently only support Rocky 8, and Ubuntu 22.04

set -e

GKE_ENABLED=$1

# Update lnet to enable GKE supported Lustre instance
if [[ $GKE_ENABLED == "1" ]]; then
	if [[ -f "/etc/modprobe.d/lnet.conf" ]] && grep -Fq "options lnet accept_port" /etc/modprobe.d/lnet.conf; then
		echo "Lnet accept port already set, continuing without updating /etc/modprobe.d/lnet.conf"
	else
		echo "options lnet accept_port=6988" >>/etc/modprobe.d/lnet.conf
	fi
fi

if grep -q lustre /proc/filesystems; then
	echo "Skipping managed lustre client install as it is already supported"
	exit 0
fi

# Get distro information
. /etc/os-release
DIST="NA"
if [[ $NAME == *"Ubuntu"* ]]; then
	if [[ $VERSION_ID == "22.04" ]]; then
		DIST="Ubuntu"
	fi
elif [[ $NAME == *"Rocky"* ]]; then
	if [[ $VERSION_ID == "8"* ]]; then
		DIST="Rocky"
	fi
fi

if [[ ${DIST} == "Ubuntu" ]]; then
	KEY_LOC=/etc/apt/keyrings
	KEY_NAME=gcp-ar-repo.gpg
	# Download new repo key
	mkdir -p "${KEY_LOC}"
	wget -O - https://us-apt.pkg.dev/doc/repo-signing-key.gpg 2>/dev/null | gpg --dearmor - | tee "${KEY_LOC}/${KEY_NAME}" >/dev/null

	# Set up apt repo
	echo "deb [ signed-by=${KEY_LOC}/${KEY_NAME} ] https://us-apt.pkg.dev/projects/lustre-client-binaries lustre-client-ubuntu-${UBUNTU_CODENAME} main" | tee -a /etc/apt/sources.list.d/artifact-registry.list

	# Install modules
	apt update
	apt install -y "lustre-client-modules-$(uname -r)" lustre-client-utils || (echo "Error finding Lustre module packages, Lustre package may not exist for this kernel version" && exit 1)
elif [[ ${DIST} == "Rocky" ]]; then
	# Set up yum repo
	touch /etc/yum.repos.d/artifact-registry.repo
	tee -a /etc/yum.repos.d/artifact-registry.repo <<-EOF
		[lustre-client-rocky-8]
		name=lustre-client-rocky-8
		baseurl=https://us-yum.pkg.dev/projects/lustre-client-binaries/lustre-client-rocky-8
		enabled=1
		repo_gpgcheck=0
		gpgcheck=0
	EOF
	# Install modules
	yum makecache
	yum --enablerepo=lustre-client-rocky-8 install -y kmod-lustre-client lustre-client
fi

if [[ $DIST != "NA" ]]; then
	# Load the new lustre client module
	modprobe lustre
fi
