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

# VirtualGL provides hardware OpenGL for backends that cannot offload it
# themselves (TurboVNC has no "-rendernode" equivalent). Its EGL back end
# renders against a DRM render node directly, so no second X server bound to the
# GPU is needed.
#
# No NVIDIA driver is installed here: use an image that already carries one, such
# as slurm-gcp-6-12-ubuntu-2204-lts-nvidia-570. Without a driver there is no
# render node and the desktop falls back to software rendering.
#
# NOTE: this file is read with file(), not templatefile(), so shell variables use
# a single "$" - "$${...}" would survive into the rendered script and bash would
# expand "$$" as the pid.
# Sourced into the runtime setup script rather than executed, so it has no
# shebang of its own.
# shellcheck shell=bash
install_virtualgl() {
	local version=3.1.4
	local sha256=02edc6b599571c385389af1a006f07a70c298e1d97c580a9bfd4b39d835c51e6
	local deb="/tmp/virtualgl_${version}_amd64.deb"

	apt-get install -y curl || return 1
	curl -fsSL --retry 5 --retry-delay 5 --retry-connrefused -o "$deb" \
		"https://github.com/VirtualGL/virtualgl/releases/download/${version}/virtualgl_${version}_amd64.deb" || return 1
	echo "${sha256}  ${deb}" | sha256sum -c - >/dev/null || return 1
	# apt-get, not dpkg -i, so the package's own dependencies are resolved.
	apt-get install -y "$deb" || return 1
	rm -f "$deb"
}

# Deliberately non-fatal. VirtualGL is an enhancement, and the broker already
# falls back to software rendering when vglrun is absent. Letting a failure here
# abort the script under "set -euo pipefail" would take down the rest of the
# runtime - the VNC server, noVNC and the broker itself - and leave no desktop at
# all rather than a slower one.
if install_virtualgl; then
	echo "VirtualGL installed; hardware OpenGL available if a render node exists."
else
	echo "WARNING: VirtualGL install failed; desktop sessions will use software rendering." >&2
fi
