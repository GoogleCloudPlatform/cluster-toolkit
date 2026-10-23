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

# Sourced into the runtime setup script rather than executed, so it has no
# shebang of its own.
# shellcheck shell=bash
dnf install -y \
	dbus-x11 \
	tigervnc-server \
	thunar \
	xauth \
	xorg-x11-fonts-Type1 \
	xfce4-panel \
	xfce4-session \
	xfce4-settings \
	xfdesktop \
	xterm
