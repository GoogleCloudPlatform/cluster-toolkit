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

CONFIGS_DIR="tools/cloud-build/daily-tests/blueprints/test-infra-kueue/configs"
PROJECTS=("hpc-toolkit-dev" "hpc-toolkit-dev-2" "hpc-toolkit-gsc")

echo "Applying Kueue Locks to Clusters..."

for PROJECT in "${PROJECTS[@]}"; do
	echo "==========================================="
	echo "Project: $PROJECT"
	echo "==========================================="

	# Check if cluster exists in this project
	if gcloud container clusters describe test-kueue-cluster --region us-central1 --project "$PROJECT" >/dev/null 2>&1; then
		echo "Getting credentials for test-kueue-cluster in $PROJECT..."
		gcloud container clusters get-credentials test-kueue-cluster --region us-central1 --project "$PROJECT"

		echo "Applying dummy-device-plugin..."
		kubectl apply -f "$CONFIGS_DIR/dummy-device-plugin.yaml"

		echo "Applying kueue-setup..."
		kubectl apply -f "$CONFIGS_DIR/kueue-setup.yaml"
	else
		echo "test-kueue-cluster not found in $PROJECT. Skipping."
	fi
	echo ""
done

echo "Done."
