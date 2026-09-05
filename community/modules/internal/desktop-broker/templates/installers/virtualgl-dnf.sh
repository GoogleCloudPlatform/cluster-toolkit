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
# No NVIDIA driver is installed here. Note that the Rocky slurm-gcp images ship
# NVIDIA userspace libraries without a driver or device, which is not enough for
# hardware GL - the published family carrying a driver is
# slurm-gcp-6-12-ubuntu-2204-lts-nvidia-570. Without a render node the desktop
# falls back to software rendering.
#
# NOTE: this file is read with file(), not templatefile(), so shell variables use
# a single "$" - "$${...}" would survive into the rendered script and bash would
# expand "$$" as the pid.
# Sourced into the runtime setup script rather than executed, so it has no
# shebang of its own.
# shellcheck shell=bash
install_virtualgl() {
	local version=3.1.4
	local sha256=349b9cacb508058a071e4414294d417d9b8db45dd35d2e9296b0ee1c6743fdbd
	local rpm="/tmp/VirtualGL-${version}.x86_64.rpm"

	dnf install -y curl || return 1
	curl -fsSL --retry 5 --retry-delay 5 --retry-connrefused -o "$rpm" \
		"https://github.com/VirtualGL/virtualgl/releases/download/${version}/VirtualGL-${version}.x86_64.rpm" || return 1
	echo "${sha256}  ${rpm}" | sha256sum -c - >/dev/null || return 1
	dnf install -y "$rpm" || return 1
	rm -f "$rpm"
}

# Deliberately non-fatal - see the apt installer for why.
if install_virtualgl; then
	echo "VirtualGL installed; hardware OpenGL available if a render node exists."
else
	echo "WARNING: VirtualGL install failed; desktop sessions will use software rendering." >&2
fi
