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

# Parse access_points.
for arg in "$@"; do
	if [[ $arg == --access_points=* ]]; then
		access_points="${arg#*=}"
	fi
done

OS_ID=$(awk -F '=' '/^ID=/ {print $2}' /etc/os-release | sed -e 's/"//g')
OS_VERSION=$(awk -F '=' '/VERSION_ID/ {print $2}' /etc/os-release | sed -e 's/"//g')
OS_VERSION_MAJOR=$(awk -F '=' '/VERSION_ID/ {print $2}' /etc/os-release | sed -e 's/"//g' -e 's/\..*$//')

if [ -x /bin/daos ]; then
	echo "DAOS already installed"
	daos version
else
	# Install the DAOS client library
	# The following commands should be executed on each client vm.
	## For Rocky linux 8 / RedHat 8.
	if [ "${OS_ID}" = "rocky" ] || [ "${OS_ID}" = "rhel" ]; then
		if [ "${OS_VERSION_MAJOR}" = "8" ]; then
			# 1) Add the Parallelstore package repository
			tee /etc/yum.repos.d/parallelstore-v2-6-el8.repo <<EOF
[parallelstore-v2-6-el8]
name=Parallelstore EL8 v2.6
baseurl=https://us-central1-yum.pkg.dev/projects/parallelstore-packages/v2-6-el8
enabled=1
repo_gpgcheck=0
gpgcheck=0
EOF
		elif [ "${OS_VERSION_MAJOR}" -eq "9" ]; then
			tee /etc/yum.repos.d/parallelstore-v2-6-el9.repo <<EOF
[parallelstore-v2-6-el9]
name=Parallelstore EL9 v2.6
baseurl=https://packages.daos.io/v2.6/EL9/packages/x86_64/
enabled=1
repo_gpgcheck=0
gpgcheck=0
EOF
		else
			echo "Unsupported RedHat / Rocky Linux system version ${OS_VERSION_MAJOR}. This script only supports version 8 and 9."
			exit 1
		fi

		## TODO: Remove disable automatic update script after issue is fixed.
		/usr/bin/google_disable_automatic_updates
		dnf makecache

		# 2) Install daos-client
		dnf install -y epel-release # needed for capstone
		dnf install -y daos-client

		# 3) Upgrade libfabric
		dnf upgrade -y libfabric

	# For Ubuntu 22.04 and debian 12,
	elif { [ "${OS_ID}" = "ubuntu" ] && [ "${OS_VERSION}" = "22.04" ]; } || { [ "${OS_ID}" = "debian" ] && [ "${OS_VERSION_MAJOR}" = "12" ]; }; then

		# 1) Add the Parallelstore package repository
		curl https://us-central1-apt.pkg.dev/doc/repo-signing-key.gpg | apt-key add -
		echo "deb https://us-central1-apt.pkg.dev/projects/parallelstore-packages v2-6-deb main" | tee -a /etc/apt/sources.list.d/artifact-registry.list

		apt update

		# 2) Install daos-client
		apt install -y daos-client

	else
		echo "Unsupported operating system ${OS_ID} ${OS_VERSION}. This script only supports Rocky Linux 8, Redhat 8, Redhat 9, Ubuntu 22.04, and Debian 12."
		exit 1
	fi
fi

# Edit agent config
daos_config=/etc/daos/daos_agent.yml
sed -i "s/#.*transport_config/transport_config/g" $daos_config
sed -i "s/#.*allow_insecure:.*false/  allow_insecure: true/g" $daos_config
sed -i "s/.*access_points.*/access_points: $access_points/g" $daos_config

# Start service
if { [ "${OS_ID}" = "rocky" ] || [ "${OS_ID}" = "rhel" ]; } && { [ "${OS_VERSION_MAJOR}" = "8" ] || [ "${OS_VERSION_MAJOR}" = "9" ]; }; then
	# TODO: Update script to change default log destination folder, after daos_agent user is supported in debian and ubuntu.
	# Move agent log destination from /tmp/ (default) to /var/log/daos_agent/
	mkdir -p /var/log/daos_agent
	chown daos_agent:daos_agent /var/log/daos_agent
	sed -i "s/#.*log_file:.*/log_file: \/var\/log\/daos_agent\/daos_agent.log/g" $daos_config
	systemctl start daos_agent.service
elif { [ "${OS_ID}" = "ubuntu" ] && [ "${OS_VERSION}" = "22.04" ]; } || { [ "${OS_ID}" = "debian" ] && [ "${OS_VERSION_MAJOR}" = "12" ]; }; then
	mkdir -p /var/run/daos_agent
	daos_agent -o /etc/daos/daos_agent.yml >/dev/null 2>&1 &
else
	echo "Unsupported operating system ${OS_ID} ${OS_VERSION}. This script only supports Rocky Linux 8, Redhat 8, Redhat 9, Ubuntu 22.04, and Debian 12."
	exit 1
fi

exit 0
