#!/bin/bash
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

set -eo pipefail

for cmd in gcloud kubectl; do
	if ! command -v "$cmd" &>/dev/null; then
		echo "Error: $cmd is required but not installed." >&2
		exit 1
	fi
done

TMP_KUBECONFIG=$(mktemp)
export KUBECONFIG="$TMP_KUBECONFIG"
trap 'rm -f "$TMP_KUBECONFIG"' EXIT

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
CONFIGS_DIR="$SCRIPT_DIR/../daily-tests/blueprints/test-infra-kueue/configs"
CLUSTER_NAME="test-kueue-cluster"
CLUSTER_REGION="us-central1"
if [ -n "$TEST_INFRA_KUEUE_PROJECTS" ]; then
	# Read space-separated projects into an array
	read -r -a PROJECTS <<<"$TEST_INFRA_KUEUE_PROJECTS"
else
	PROJECTS=("hpc-toolkit-dev" "hpc-toolkit-dev-2" "hpc-toolkit-gsc")
fi

echo "Applying Kueue Locks to Clusters..."

ERRORS=0
for PROJECT in "${PROJECTS[@]}"; do
	echo "==========================================="
	echo "Project: $PROJECT"
	echo "==========================================="

	cluster_found=$(gcloud container clusters list --region "$CLUSTER_REGION" --project "$PROJECT" --filter="name=$CLUSTER_NAME" --format="value(name)")
	if [ "$cluster_found" == "$CLUSTER_NAME" ]; then
		echo "Getting credentials for $CLUSTER_NAME in $PROJECT..."
		if gcloud container clusters get-credentials "$CLUSTER_NAME" --region "$CLUSTER_REGION" --project "$PROJECT"; then
			echo "Applying dummy-device-plugin..."
			kubectl apply -f "$CONFIGS_DIR/dummy-device-plugin.yaml" || ERRORS=1

			echo "Applying kueue-setup..."
			kubectl apply -f "$CONFIGS_DIR/kueue-setup.yaml" || ERRORS=1
		else
			echo "Failed to get credentials for $CLUSTER_NAME in $PROJECT."
			ERRORS=1
		fi
	else
		echo "$CLUSTER_NAME not found in $PROJECT. Skipping."
	fi
	echo ""
done

if [ "$ERRORS" -ne 0 ]; then
	echo "Error: Failed to apply Kueue locks to one or more clusters."
	exit 1
fi

echo "Done."
