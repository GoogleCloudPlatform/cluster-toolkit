#!/usr/bin/env bash
# Copyright 2026 Google LLC
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

set -euo pipefail

if [ -x /opt/venv/bin/pip ]; then
	/opt/venv/bin/pip install --no-cache-dir "pyyaml>=6.0.1"
else
	pip3 install --no-cache-dir "pyyaml>=6.0.1"
fi

if [ -x /usr/bin/python3 ] && ! /usr/bin/python3 -c "import yaml" 2>/dev/null; then
	apt-get -y update && apt-get -y install --no-install-recommends python3-yaml
fi

if ! command -v npm >/dev/null 2>&1; then
	apt-get -y update && apt-get -y install --no-install-recommends ca-certificates curl gnupg
	mkdir -p /usr/share/keyrings
	curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key |
		gpg --batch --yes --dearmor -o /usr/share/keyrings/nodesource.gpg
	echo "deb [signed-by=/usr/share/keyrings/nodesource.gpg] https://deb.nodesource.com/node_22.x nodistro main" \
		>/etc/apt/sources.list.d/nodesource.list
	apt-get -y update && apt-get -y install --no-install-recommends nodejs
fi

if ! gemini --version 2>/dev/null | grep -qx '0\.68\.0'; then
	npm install -g @google/gemini-cli@0.68.0
	npm cache clean --force
	rm -rf ${HOME:+"$HOME/.npm"} /root/.npm
fi

apt-get clean
rm -rf ${HOME:+"$HOME/.cache/pip"} /root/.cache/pip /var/lib/apt/lists/*
