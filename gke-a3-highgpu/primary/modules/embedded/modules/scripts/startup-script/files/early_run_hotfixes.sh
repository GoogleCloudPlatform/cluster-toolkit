#!/bin/bash
# Copyright 2024 Google LLC
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

# This script applies fixes to VMs that must occur early in boot. For example,
# when yum or apt repositories are misconfigured, preventing most package
# operations from completing successfully.

source /etc/os-release

if [[ "$PRETTY_NAME" == "CentOS Linux 7 (Core)" ]]; then
	echo "Applying hotfixes for CentOS 7"
	if grep -q '^mirrorlist' /etc/yum.repos.d/CentOS-Base.repo; then
		echo "Removing mirrorlist from default CentOS 7 repositories"
		sed -i '/^mirrorlist/d' /etc/yum.repos.d/CentOS-Base.repo
	fi
	if grep -q '^#baseurl=http://mirror.centos.org' /etc/yum.repos.d/CentOS-Base.repo; then
		echo "Reconfiguring default CentOS 7 repositories to use CentOS Vault"
		sed -i 's,^#baseurl=http://mirror.centos.org/,baseurl=http://vault.centos.org/,' /etc/yum.repos.d/CentOS-Base.repo
	fi
fi
