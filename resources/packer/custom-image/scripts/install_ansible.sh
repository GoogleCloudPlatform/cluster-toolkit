#!/bin/sh
# Copyright 2022 Google LLC
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

apt_wait() {
	while fuser /var/lib/dpkg/lock >/dev/null 2>&1; do
		echo "Sleeping for dpkg lock"
		sleep 3
	done
	while fuser /var/lib/apt/lists/lock >/dev/null 2>&1; do
		echo "Sleeping for apt lists lock"
		sleep 3
	done
	if [ -f /var/log/unattended-upgrades/unattended-upgrades.log ]; then
		echo "Sleeping until unattended-upgrades finishes"
		while fuser /var/log/unattended-upgrades/unattended-upgrades.log >/dev/null 2>&1; do
			sleep 3
		done
	fi
}

if ! command -v ansible-playbook >/dev/null 2>&1; then
	if [ -f /etc/centos-release ] || [ -f /etc/redhat-release ] || [ -f /etc/oracle-release ] || [ -f /etc/system-release ]; then
		yum -y install epel-release
		yum -y install ansible

	elif [ -f /etc/debian_version ] || grep -qi ubuntu /etc/lsb-release || grep -qi ubuntu /etc/os-release; then
		echo 'WARNING: unsupported installation of ansible in debian / ubuntu'
		apt_wait
		apt-get update
		DEBIAN_FRONTEND=noninteractive apt-get install -y ansible
	else
		echo 'Unsupported distribution'
		exit 1
	fi
fi
