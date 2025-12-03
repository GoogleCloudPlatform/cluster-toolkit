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

BUILD_ID_SHORT="${BUILD_ID:0:6}"
PROVISIONING_MODEL="SPOT"
TERMINATION_ACTION="DELETE"
OPTIONS_GCS_PATH=""
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
		local INSTANCES_TO_DELETE_ARRAY=()
		readarray -t INSTANCES_TO_DELETE_ARRAY < <(gcloud compute instances list --project="${project}" --zones="${zone}" \
			--filter="name ~ ^${prefix}" --format='value(name)')

		if [[ "${#INSTANCES_TO_DELETE_ARRAY[@]}" -gt 0 ]]; then
			DELETE_OUTPUT=$(gcloud compute instances delete "${INSTANCES_TO_DELETE_ARRAY[@]}" \
				--project="${project}" \
				--zone="${zone}" \
				--quiet \
				--delete-disks=all 2>&1)
			delete_status=$?
			if [[ ${delete_status} -ne 0 ]]; then
				echo "ERROR IN DELETING RESOURCE: ${DELETE_OUTPUT}"
			fi
		fi
	fi
}

if [[ "${MACHINE_TYPE}" == "a3-ultragpu-8g" ]]; then
	OPTIONS_GCS_PATH="gs://hpc-ctk1357/a3uoptions.txt"
elif [[ "${MACHINE_TYPE}" == "a3-megagpu-8g" ]]; then
	OPTIONS_GCS_PATH="gs://hpc-ctk1357/a3moptions.txt"
else
	echo "ERROR: Failed to find file corresponding to machine type."
	set -e
	exit 1
fi

GCS_CONTENT=$(gcloud storage cat "${OPTIONS_GCS_PATH}" 2>&1)
GCLOUD_CAT_EXIT_CODE=$?
if [[ ${GCLOUD_CAT_EXIT_CODE} -ne 0 ]]; then
	echo "ERROR: Failed to read ${OPTIONS_GCS_PATH}."
	set -e
	exit 1
fi

declare -a ZONES_ARRAY=()
while IFS= read -r line; do
	if [[ -n "${line}" ]]; then
		ZONES_ARRAY+=("${line}")
	fi
done <<<"${GCS_CONTENT}"

if [[ "${#ZONES_ARRAY[@]}" -eq 0 ]]; then
	echo "ERROR: No valid zones found in ${OPTIONS_GCS_PATH}"
	set -e
	exit 1
fi

SELECTED_REGION=""
SELECTED_ZONE=""
SUCCESS=false

# Loop through all zones to find capacity
for ZONE in "${ZONES_ARRAY[@]}"; do
	REGION="${ZONE%-*}"
	readarray -t INSTANCE_NAMES_ARRAY < <(generate_instance_names "${FULL_INSTANCE_PREFIX}" "${NUM_NODES}")

	declare -a GCLOUD_CMD
	COMMON_FLAGS=(
		--project="${PROJECT_ID}"
		--zone="${ZONE}"
		--machine-type="${MACHINE_TYPE}"
		--image-project="${IMAGE_PROJECT}"
		--provisioning-model="${PROVISIONING_MODEL}"
		--instance-termination-action="${TERMINATION_ACTION}"
		--quiet
	)

	if [[ "${MACHINE_TYPE}" == "a3-ultragpu-8g" ]]; then
		if [[ -z "${IMAGE_NAME}" ]]; then
			echo "ERROR: IMAGE_NAME must be set for a3-ultragpu-8g" >&2
			set -e
			exit 1
		fi
		GCLOUD_CMD=(
			gcloud compute instances create "${INSTANCE_NAMES_ARRAY[@]}"
			"${COMMON_FLAGS[@]}"
			--image="${IMAGE_NAME}"
		)
	else
		if [[ -z "${IMAGE_FAMILY}" ]]; then
			echo "ERROR: IMAGE_FAMILY must be set for ${MACHINE_TYPE}" >&2
			set -e
			exit 1
		fi
		GCLOUD_CMD=(
			gcloud compute instances create "${INSTANCE_NAMES_ARRAY[@]}"
			"${COMMON_FLAGS[@]}"
			--image-family="${IMAGE_FAMILY}"
			--no-address
		)
	fi

	CREATE_OUTPUT=$("${GCLOUD_CMD[@]}" 2>&1)
	GCLOUD_EXIT_CODE=$?
	cleanup_instances "${PROJECT_ID}" "${ZONE}" "${FULL_INSTANCE_PREFIX}"

	if [[ ${GCLOUD_EXIT_CODE} -eq 0 ]]; then
		SELECTED_REGION="${REGION}"
		SELECTED_ZONE="${ZONE}"
		SUCCESS=true
		break
	elif [[ "${CREATE_OUTPUT}" != *"INSUFFICIENT_CAPACITY"* &&
		"${CREATE_OUTPUT}" != *"ZONE_RESOURCE_POOL_EXHAUSTED"* ]]; then
		echo "ERROR: Unexpected error ${CREATE_OUTPUT} (Exit Code: ${GCLOUD_EXIT_CODE})." >&2
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
		--extra-vars="project=${PROJECT_ID} build=${BUILD_ID_SHORT}" \
		--extra-vars="region=${SELECTED_REGION} zone=${SELECTED_ZONE}" \
		--extra-vars="${TEST_VARS_FILE}"

	echo "--- DEPLOYMENT COMPLETE ---"
else
	echo "--- DEPLOYMENT FAILED ---" >&2
	set -e
	exit 1
fi
