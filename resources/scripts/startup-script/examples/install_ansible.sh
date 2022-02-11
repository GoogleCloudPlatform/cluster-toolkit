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

if [ ! "$(which ansible-playbook)" ]; then
	if [ -f /etc/centos-release ] || [ -f /etc/redhat-release ] || [ -f /etc/oracle-release ] || [ -f /etc/system-release ]; then
		yum -y install epel-release
		yum -y install ansible

	elif [ -f /etc/debian_version ] || grep -qi ubuntu /etc/lsb-release || grep -qi ubuntu /etc/os-release; then
		echo 'WARNING: unsupported installation in debian / ubuntu'
		apt update
		apt install -y software-properties-common
		add-apt-repository --yes --update ppa:ansible/ansible
		apt install -y ansible

	else
		echo 'Unsupported distribution'
		exit 1
	fi
fi
