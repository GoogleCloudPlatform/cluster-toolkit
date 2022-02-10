#!/bin/bash
# Copyright 2021 Google LLC
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

RUNNER_DIR=$(dirname "$0")

echo "Pausing for startup script to finish..."
sleep 60

echo "Creating inventory..."
mkdir -p "${RUNNER_DIR}/ghpc-install/data"
python3 "${RUNNER_DIR}/create_inventory.py" \
	--deployment_name "${DEPLOYMENT_NAME}" \
	--template "${RUNNER_DIR}/inventory.tmpl" \
	--outfile "${RUNNER_DIR}/ghpc-install/data/inventory" \
	--project "${PROJECT_ID}" \
	--zone "${ZONE}"

echo "Copying runner data to Omnia Manager..."
gcloud compute scp --project "${PROJECT_ID}" --zone "${ZONE}" \
	--recurse "${RUNNER_DIR}/ghpc-install" "${MANAGER_NODE}":

echo "Applying the Omnia runner..."
gcloud compute ssh \
	--project "${PROJECT_ID}" --zone "${ZONE}" "${MANAGER_NODE}" \
	-- "cd ~/ghpc-install/scripts; source install_omnia.sh"
