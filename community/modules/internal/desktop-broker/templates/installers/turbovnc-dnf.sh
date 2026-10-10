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
# verified, rather than from a third-party yum repository.
#
# Upstream's alternative is a .repo file fetched from raw.githubusercontent.com,
# which pins nothing and brings its own signing key. A single pinned .rpm makes
# the installed version explicit and needs no repository or key.
#
# Sourced into the runtime setup script rather than executed, so it has no
# shebang of its own.
# shellcheck shell=bash
install_turbovnc() {
	local version=3.3.1
	local sha256=cb975ccc3570f35d7b47fb56d4e2f7d5121418e3e6a831a3f24b831251ddbc4b
	local rpm="/tmp/turbovnc-${version}.x86_64.rpm"

	curl -fsSL --retry 5 --retry-delay 5 --retry-connrefused -o "$rpm" \
		"https://github.com/TurboVNC/turbovnc/releases/download/${version}/turbovnc-${version}.x86_64.rpm"
	echo "${sha256}  ${rpm}" | sha256sum -c - >/dev/null
	dnf install -y "$rpm"
	rm -f "$rpm"
}

dnf install -y curl
install_turbovnc

dnf install -y \
	dbus-x11 \
	thunar \
	xauth \
	xorg-x11-fonts-Type1 \
	xfce4-panel \
	xfce4-session \
	xfce4-settings \
	xfdesktop \
	xterm
