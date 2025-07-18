#!/bin/bash
# Copyright 2024 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#	   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e -o pipefail

web_proxy="${1:-}"
if [ -z "$web_proxy" ]; then
	echo "Error: must provide 1 argument identifying http/https proxy"
	exit 1
fi

# configure pip to use proxy
PIP_CONF=/etc/pip.conf
if [ ! -f "$PIP_CONF" ]; then
	cat <<-EOF >"$PIP_CONF"
		[global]
		proxy=$web_proxy
	EOF
fi

# configure yum or dnf to use proxy
if [ -f /etc/centos-release ] || [ -f /etc/redhat-release ] ||
	[ -f /etc/oracle-release ] || [ -f /etc/system-release ]; then
	YUM_CONF="/etc/yum.conf"
	if ! grep -q '^proxy=.*' "$YUM_CONF"; then
		sed --follow-symlinks -i.bak "/^\[main]/a proxy=$web_proxy" "$YUM_CONF"
	else
		sed --follow-symlinks -i.bak "s,proxy=.*,proxy=$web_proxy," "$YUM_CONF"
	fi
fi

# configure apt to use proxy
if [ -f /etc/debian_version ] || grep -qi ubuntu /etc/lsb-release 2>/dev/null ||
	grep -qi ubuntu /etc/os-release 2>/dev/null; then
	APT_CONF_PROXY="/etc/apt/apt.conf.d/99proxy.conf"
	if [ ! -f "$APT_CONF_PROXY" ]; then
		cat <<-EOF >"$APT_CONF_PROXY"
			Acquire::http::Proxy "$web_proxy";
			Acquire::https::Proxy "$web_proxy";
		EOF
	fi
fi
