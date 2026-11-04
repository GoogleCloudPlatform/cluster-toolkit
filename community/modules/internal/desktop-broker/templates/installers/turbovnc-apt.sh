# Copyright 2026 "Google LLC"
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

# TurboVNC is installed from its published release package, pinned and checksum
# verified, rather than from a third-party apt repository.
#
# Upstream distributes through packagecloud.io, which means adding a repository
# and a signing key. That was actively harmful here: the key has to be handled
# without invoking gpg, because "gpg --dearmor" aborts on OFE-provisioned nodes.
# gpg resolves the calling user through NSS at startup, and when it runs just
# after the host's NSS configuration has been rewritten (OS Login/SSSD) it dies
# with "Illegal status in __nss_next". That is SIGABRT, so under "set -euo
# pipefail" the whole runtime install failed with exit 134 and the node came up
# with no desktop at all. The failure is timing-dependent, so it could not be
# reproduced by hand.
#
# A single pinned .deb removes the repository, the key, and that entire failure
# mode, and makes the installed version explicit rather than "whatever the
# repository serves today".
#
# Sourced into the runtime setup script rather than executed, so it has no
# shebang of its own.
# shellcheck shell=bash
install_turbovnc() {
	local version=3.3.1
	local sha256=5d99050312360a07c28aca484b944816aa32e9fe07912e3ed50e6bfda45f80ad
	local deb="/tmp/turbovnc_${version}_amd64.deb"

	curl -fsSL --retry 5 --retry-delay 5 --retry-connrefused -o "$deb" \
		"https://github.com/TurboVNC/turbovnc/releases/download/${version}/turbovnc_${version}_amd64.deb"
	echo "${sha256}  ${deb}" | sha256sum -c - >/dev/null
	# apt-get, not dpkg -i, so the package's own dependencies are resolved.
	apt-get install -y "$deb"
	rm -f "$deb"
}

apt-get install -y curl
install_turbovnc

apt-get install -y \
	dbus-x11 \
	xauth \
	xfce4 \
	xterm
