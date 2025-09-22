#!/bin/bash
# Copyright 2024 Google LLC
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

# This script updates the local Kubernetes manifest files from the official
# gateway-api-inference-extension repository.

set -e

# Get the directory of the script
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

echo "Downloading latest manifests to ${SCRIPT_DIR}..."

# URLs for the manifest files
GPU_DEPLOYMENT_URL="https://github.com/kubernetes-sigs/gateway-api-inference-extension/raw/main/config/manifests/vllm/gpu-deployment.yaml"
MANIFESTS_URL="https://github.com/kubernetes-sigs/gateway-api-inference-extension/releases/latest/download/manifests.yaml"
GATEWAY_URL="https://github.com/kubernetes-sigs/gateway-api-inference-extension/raw/main/config/manifests/gateway/gke/gateway.yaml"
HTTPROUTE_URL="https://github.com/kubernetes-sigs/gateway-api-inference-extension/raw/main/config/manifests/gateway/gke/httproute.yaml"
INFERENCEOBJECTIVE_URL="https://github.com/kubernetes-sigs/gateway-api-inference-extension/raw/main/config/manifests/inferenceobjective.yaml"

# Download and overwrite local files
curl -sL "${GPU_DEPLOYMENT_URL}" > "${SCRIPT_DIR}/gpu-deployment.yaml"
echo "Updated gpu-deployment.yaml"

curl -sL "${MANIFESTS_URL}" > "${SCRIPT_DIR}/manifests.yaml"
echo "Updated manifests.yaml"

curl -sL "${GATEWAY_URL}" > "${SCRIPT_DIR}/gateway.yaml"
echo "Updated gateway.yaml"

curl -sL "${HTTPROUTE_URL}" > "${SCRIPT_DIR}/httproute.yaml"
echo "Updated httproute.yaml"

curl -sL "${INFERENCEOBJECTIVE_URL}" > "${SCRIPT_DIR}/inferenceobjective.yaml"
echo "Updated inferenceobjective.yaml"

echo "All manifest files have been updated successfully."
