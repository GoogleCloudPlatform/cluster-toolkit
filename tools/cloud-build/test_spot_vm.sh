#!/bin/bash
# Copyright 2025 "Google LLC"
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

set +e

PROJECT=$PROJECT_ID
BUILD_ID_FULL=$BUILD_ID
BUILD_ID_SHORT="${BUILD_ID_FULL:0:6}"
IMAGE_PROJECT=$PROJECT_ID
PROVISIONING_MODEL="SPOT"
TERMINATION_ACTION="DELETE"
OPTIONS_GCS_PATH="gs://hpc-ctk1357/options/options.txt"
FULL_INSTANCE_PREFIX="${INSTANCE_PREFIX}-${BUILD_ID_SHORT}-"

generate_instance_names() {
	local prefix=$1
	local num=$2
	for i in $(seq -w 01 "$num"); do
		echo "${prefix}${i}"
	done
}

cleanup_instances() {
	local project=$1
	local zone=$2
	local prefix=$3

	if [[ -n "${zone}" ]]; then
		local INSTANCES_TO_DELETE
		INSTANCES_TO_DELETE=$(gcloud compute instances list --project="${project}" --zones="${zone}" \
			--filter="name ~ ^${prefix}" --format='value(name)')

		if [[ -n "${INSTANCES_TO_DELETE}" ]]; then
			echo "${INSTANCES_TO_DELETE}" | while read -r inst; do
				gcloud compute instances delete "${inst}" --project="${project}" --zone="${zone}" --quiet --delete-disks=all || true
			done
		fi
	fi
}

GCS_CONTENT=$(gcloud storage cat "${OPTIONS_GCS_PATH}" 2>&1)
GCLOUD_CAT_EXIT_CODE=$?
if [[ ${GCLOUD_CAT_EXIT_CODE} -ne 0 ]]; then
	echo "--- ERROR: Failed to read ${OPTIONS_GCS_PATH}."
	set -e
	exit 1
fi

declare -a REGION_ZONE_PAIRS=()
while IFS= read -r line; do
	if [[ -n "${line}" ]]; then
		REGION_ZONE_PAIRS+=("${line}")
	fi
done <<<"${GCS_CONTENT}"

if [[ "${#REGION_ZONE_PAIRS[@]}" -eq 0 ]]; then
	echo "--- ERROR: No valid region/zone pairs found in ${OPTIONS_GCS_PATH} ---"
	set -e
	exit 1
fi

SELECTED_REGION=""
SELECTED_ZONE=""
SUCCESS=false

for pair in "${REGION_ZONE_PAIRS[@]}"; do
	REGION=$(echo "${pair}" | awk '{print $1}')
	ZONE=$(echo "${pair}" | awk '{print $2}')

	readarray -t INSTANCE_NAMES_ARRAY < <(generate_instance_names "${FULL_INSTANCE_PREFIX}" "${NUM_NODES}")

	CREATE_OUTPUT=$(gcloud compute instances create "${INSTANCE_NAMES_ARRAY[@]}" \
		--project="${PROJECT}" \
		--zone="${ZONE}" \
		--machine-type="${MACHINE_TYPE}" \
		--image-family="${IMAGE_FAMILY}" \
		--image-project="${IMAGE_PROJECT}" \
		--provisioning-model="${PROVISIONING_MODEL}" \
		--instance-termination-action="${TERMINATION_ACTION}" \
		--no-address \
		--quiet 2>&1)

	GCLOUD_EXIT_CODE=$?

	if [[ ${GCLOUD_EXIT_CODE} -ne 0 ]] || [[ "${CREATE_OUTPUT}" == *"ERROR:"* ]] || [[ "${CREATE_OUTPUT}" == *"Could not fetch resource:"* ]]; then
		cleanup_instances "${PROJECT}" "${ZONE}" "${FULL_INSTANCE_PREFIX}"
	else
		cleanup_instances "${PROJECT}" "${ZONE}" "${FULL_INSTANCE_PREFIX}"
		SELECTED_REGION="${REGION}"
		SELECTED_ZONE="${ZONE}"
		SUCCESS=true
		break
	fi
done

if [[ "${SUCCESS}" == "true" ]]; then
	set -e
	echo "Deploying in location: REGION=${SELECTED_REGION}, ZONE=${SELECTED_ZONE}"
	cd /workspace && make

	sed -i -e '/deletion_protection:/{n;s/enabled: true/enabled: false/}' "${BLUEPRINT_PATH}"
	sed -i -e '/reason:/d' "${BLUEPRINT_PATH}"

	ansible-playbook tools/cloud-build/daily-tests/ansible_playbooks/slurm-integration-test.yml \
		--user=sa_106486320838376751393 \
		--extra-vars="project=${PROJECT} build=${BUILD_ID_SHORT}" \
		--extra-vars="region=${SELECTED_REGION} zone=${SELECTED_ZONE}" \
		--extra-vars="${TEST_VARS_FILE}"

	echo "--- DEPLOYMENT COMPLETE ---"
else
	echo "--- DEPLOYMENT FAILED (No Capacity Found) ---" >&2
	set -e
	exit 1
fi
