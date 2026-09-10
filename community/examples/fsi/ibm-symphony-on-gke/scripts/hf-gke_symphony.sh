#!/bin/bash
# Copyright 2026 Google LLC
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
set -o pipefail
set -x

export EGO_TOP=$1

git clone https://github.com/google/symphony-gcp.git
cd symphony-gcp/hf-provider || exit 1

curl -LsSf https://astral.sh/uv/0.11.26/install.sh | sh

# shellcheck source=/dev/null
source /root/.local/bin/env
uv venv
# shellcheck source=/dev/null
source .venv/bin/activate
uv pip install .
uv pip install pyinstaller
PYTHONPATH=src pyinstaller --onefile src/gke_provider/__main__.py --name hf-gke --paths .venv/lib/python*/site-packages

cp dist/hf-gke resources/gke_cli/1.2/providerplugins/gcpgke/bin/
cd resources/gke_cli || exit 1

# Create deployment tar
tar czf hf-gke.tgz ./*

# Get Symphony Environment
# shellcheck source=/dev/null
source "${EGO_TOP}/profile.platform"

# Untar in Hostfactory
tar -xzf hf-gke.tgz -C "$HF_TOP"

# Update Config
cp /tmp/Symphony/hostProviderPlugins.json "$HF_TOP/conf/providerplugins/hostProviderPlugins.json"
cp /tmp/Symphony/hostProviders.json "$HF_TOP/conf/providers/hostProviders.json"
cp /tmp/Symphony/hostRequestors.json "$HF_TOP/conf/requestors/hostRequestors.json"

sed -i -e "s|MANUAL|AUTOMATIC|g" "$EGO_ESRVDIR/esc/conf/services/hostfactory.xml"
