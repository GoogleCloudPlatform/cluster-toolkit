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

set -e

# Parse access_points.
for arg in "$@"; do
	if [[ $arg == --access_points=* ]]; then
		access_points="${arg#*=}"
	fi
done

# Install the DAOS client library
# The following commands should be executed on each client vm.
## For Rocky linux 8.
if grep -q "ID=\"rocky\"" /etc/os-release && lsb_release -rs | grep -q "8\.[0-9]"; then

	# 1) Add the Parallelstore package repository
	tee /etc/yum.repos.d/parallelstore-v2-4-el8.repo <<EOF
[parallelstore-v2-4-el8]
name=Parallelstore EL8 v2.4
baseurl=https://us-central1-yum.pkg.dev/projects/parallelstore-packages/v2-4-el8
enabled=1
repo_gpgcheck=0
gpgcheck=0
EOF
	dnf makecache

	# 2) Install daos-client
	dnf install -y epel-release # needed for capstone
	dnf install -y daos-client

	# 3) Upgrade libfabric
	dnf upgrade -y libfabric

# For Ubuntu 22.04 and debian 12,
elif (grep -q "ID=ubuntu" /etc/os-release && lsb_release -rs | grep -q "22\.04") || (grep -q "ID=debian" /etc/os-release && lsb_release -rs | grep -q "12"); then

	# 1) Add the Parallelstore package repository
	curl https://us-central1-apt.pkg.dev/doc/repo-signing-key.gpg | apt-key add -
	echo "deb https://us-central1-apt.pkg.dev/projects/parallelstore-packages v2-4-deb main" | tee -a /etc/apt/sources.list.d/artifact-registry.list

	apt update

	# 2) Install daos-client
	apt install -y daos-client

else
	echo "Unsupported operating system. This script only supports Rocky Linux 8, Ubuntu 22.04, and Debian 12."
	exit 1
fi

# Edit agent config
daos_config=/etc/daos/daos_agent.yml
sed -i "s/#.*transport_config/transport_config/g" $daos_config
sed -i "s/#.*allow_insecure:.*false/  allow_insecure: true/g" $daos_config
sed -i "s/.*access_points.*/access_points: $access_points/g" $daos_config

# Start service
if grep -q "ID=\"rocky\"" /etc/os-release && lsb_release -rs | grep -q "8\.[0-9]"; then
	systemctl start daos_agent.service

elif (grep -q "ID=ubuntu" /etc/os-release && lsb_release -rs | grep -q "22\.04") || (grep -q "ID=debian" /etc/os-release && lsb_release -rs | grep -q "12"); then
	mkdir -p /var/run/daos_agent
	daos_agent -o /etc/daos/daos_agent.yml &

else
	echo "Unsupported operating system. This script only supports Rocky Linux 8, Ubuntu 22.04, and Debian 12."
	exit 1
fi

exit 0
