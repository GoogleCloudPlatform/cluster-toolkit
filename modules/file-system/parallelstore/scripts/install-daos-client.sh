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
else
	# Install the DAOS client library
	# The following commands should be executed on each client vm.
	## For Rocky linux 8 / RedHat 8.
	if [ "${OS_ID}" = "rocky" ] || [ "${OS_ID}" = "rhel" ]; then
		# 1) Add the Parallelstore package repository
		cat >/etc/yum.repos.d/parallelstore-v2-6-el"${OS_VERSION_MAJOR}".repo <<-EOF
			[parallelstore-v2-6-el${OS_VERSION_MAJOR}]
			name=Parallelstore EL${OS_VERSION_MAJOR} v2.6
			baseurl=https://us-central1-yum.pkg.dev/projects/parallelstore-packages/v2-6-el${OS_VERSION_MAJOR}
			enabled=1
			repo_gpgcheck=0
			gpgcheck=0
		EOF

		## TODO: Remove disable automatic update script after issue is fixed.
		if [ -x /usr/bin/google_disable_automatic_updates ]; then
			/usr/bin/google_disable_automatic_updates
		fi
		dnf clean all
		dnf makecache

		# 2) Install daos-client
		dnf install -y epel-release # needed for capstone
		dnf install -y daos-client

		# 3) Upgrade libfabric
		dnf upgrade -y libfabric

	# For Ubuntu 22.04 and debian 12,
	elif [[ "${OS_ID}" = "ubuntu" ]] || [[ "${OS_ID}" = "debian" ]]; then
		# shellcheck disable=SC2034
		DEBIAN_FRONTEND=noninteractive

		# 1) Add the Parallelstore package repository
		curl -o /etc/apt/trusted.gpg.d/us-central1-apt.pkg.dev.asc https://us-central1-apt.pkg.dev/doc/repo-signing-key.gpg
		echo "deb https://us-central1-apt.pkg.dev/projects/parallelstore-packages v2-6-deb main" >/etc/apt/sources.list.d/artifact-registry.list

		apt-get update

		# 2) Install daos-client
		apt-get install -y daos-client

		# 3) Create daos_agent.service (comes pre-installed with RedHat)
		if ! getent passwd daos_agent >/dev/null 2>&1; then
			useradd daos_agent
		fi
		cat >/etc/systemd/system/daos_agent.service <<-EOF
			[Unit]
			Description=DAOS Agent
			StartLimitIntervalSec=60
			Wants=network-online.target
			After=network-online.target

			[Service]
			Type=notify
			User=daos_agent
			Group=daos_agent
			RuntimeDirectory=daos_agent
			RuntimeDirectoryMode=0755
			ExecStart=/usr/bin/daos_agent -o /etc/daos/daos_agent.yml
			StandardOutput=journal
			StandardError=journal
			Restart=always
			RestartSec=10
			LimitMEMLOCK=infinity
			LimitCORE=infinity
			StartLimitBurst=5

			[Install]
			WantedBy=multi-user.target
		EOF
	else
		echo "Unsupported operating system ${OS_ID} ${OS_VERSION}. This script only supports Rocky Linux 8, Redhat 8, Redhat 9, Ubuntu 22.04, and Debian 12."
		exit 1
	fi
fi

exit 0
