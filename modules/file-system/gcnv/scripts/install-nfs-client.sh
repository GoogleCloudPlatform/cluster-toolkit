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

if [ ! "$(which mount.nfs)" ]; then
	if [ -f /etc/centos-release ] || [ -f /etc/redhat-release ] ||
		[ -f /etc/oracle-release ] || [ -f /etc/system-release ]; then
		major_version=$(rpm -E "%{rhel}")
		enable_repo=""
		if [ "${major_version}" -eq "7" ]; then
			enable_repo="base,epel"
		elif [ "${major_version}" -eq "8" ]; then
			enable_repo="baseos"
		else
			echo "Unsupported version of centos/RHEL/Rocky"
			return 1
		fi
		yum install --disablerepo="*" --enablerepo=${enable_repo} -y nfs-utils
	elif [ -f /etc/debian_version ] || grep -qi ubuntu /etc/lsb-release || grep -qi ubuntu /etc/os-release; then
		apt-get update --allow-releaseinfo-change-origin --allow-releaseinfo-change-label
		apt-get -y install nfs-common
	else
		echo 'Unsuported distribution'
		return 1
	fi
fi
