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

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
CONFIGS_DIR="$SCRIPT_DIR/../daily-tests/blueprints/test-infra-kueue/configs"
CLUSTER_NAME="test-kueue-cluster"
CLUSTER_REGION="us-central1"
PROJECTS=("hpc-toolkit-dev" "hpc-toolkit-dev-2" "hpc-toolkit-gsc")

echo "Applying Kueue Locks to Clusters..."

ERRORS=0
for PROJECT in "${PROJECTS[@]}"; do
	echo "==========================================="
	echo "Project: $PROJECT"
	echo "==========================================="

	# Check if cluster exists in this project
	if gcloud container clusters describe "$CLUSTER_NAME" --region "$CLUSTER_REGION" --project "$PROJECT" >/dev/null 2>&1; then
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
