#!/bin/sh
# Copyright 2023 Google LLC
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

if [ ! "$(which gcsfuse)" ]; then
	if [ -f /etc/centos-release ] || [ -f /etc/redhat-release ]; then
		tee /etc/yum.repos.d/gcsfuse.repo >/dev/null <<EOF
[gcsfuse]
name=gcsfuse (packages.cloud.google.com)
baseurl=https://packages.cloud.google.com/yum/repos/gcsfuse-el7-x86_64
enabled=1
gpgcheck=1
repo_gpgcheck=0
gpgkey=https://packages.cloud.google.com/yum/doc/yum-key.gpg
    https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg
EOF
		yum -y install gcsfuse

	elif [ -f /etc/debian_version ] || grep -qi ubuntu /etc/lsb-release || grep -qi ubuntu /etc/os-release; then
		RELEASE=$(lsb_release -c -s)
		export GCSFUSE_REPO="gcsfuse-${RELEASE}"
		echo "deb https://packages.cloud.google.com/apt $GCSFUSE_REPO main" | sudo tee /etc/apt/sources.list.d/gcsfuse.list
		curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -

		apt-get update --allow-releaseinfo-change-origin --allow-releaseinfo-change-label
		apt-get -y install gcsfuse
	else
		echo 'Unsuported distribution'
		return 1
	fi
fi
