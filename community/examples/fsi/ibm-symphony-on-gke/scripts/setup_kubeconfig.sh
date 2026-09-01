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
set -o pipefail
set -x

export EGO_TOP=$1
export PROJECT_ID=$2
export REGION=$3
export CLUSTER_NAME=$4

source $EGO_TOP/profile.platform

mkdir -p $HF_TOP/conf/providers/gcpgkeinst

# Install gke-gcloud-auth-plugin if not present
if ! command -v gke-gcloud-auth-plugin &> /dev/null; then
  dnf install -y google-cloud-cli-gke-gcloud-auth-plugin || true
fi
# Install kubectl if not present
if ! command -v kubectl &> /dev/null; then
  dnf install -y kubectl || true
fi

# Generate kubeconfig using gcloud with retry and timeout
export KUBECONFIG=$HF_TOP/conf/providers/gcpgkeinst/kubeconfig

TIMEOUT_SECONDS=${CLUSTER_WAIT_TIMEOUT:-900}  # 15 minutes default (GKE creation typically takes 4-8 min)
INTERVAL=15
DEADLINE=$((SECONDS + TIMEOUT_SECONDS))

echo "Waiting for GKE cluster '$CLUSTER_NAME' in '$REGION' to become RUNNING (timeout: ${TIMEOUT_SECONDS}s)..."

while true; do
  STATUS=$(gcloud container clusters describe "$CLUSTER_NAME" \
    --region "$REGION" \
    --project "$PROJECT_ID" \
    --format="value(status)" 2>/dev/null || true)

  if [[ "$STATUS" == "RUNNING" ]]; then
    echo "Cluster '$CLUSTER_NAME' is RUNNING."
    break
  fi

  if (( SECONDS >= DEADLINE )); then
    echo "ERROR: Timed out waiting for cluster '$CLUSTER_NAME' after ${TIMEOUT_SECONDS}s. Last status: '${STATUS:-NOT_FOUND}'" >&2
    exit 1
  fi

  echo "Cluster status: '${STATUS:-NOT_FOUND}'. Retrying in ${INTERVAL}s..."
  sleep "$INTERVAL"
done

gcloud container clusters get-credentials "$CLUSTER_NAME" --region "$REGION" --project "$PROJECT_ID"
chmod 644 "$KUBECONFIG"

