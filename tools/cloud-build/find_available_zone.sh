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
FULL_INSTANCE_PREFIX="${INSTANCE_PREFIX}-${BUILD_ID_SHORT}-"
MIN_NODES=2 # Define minimum number of nodes required

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

	local instance_list_output
	if ! instance_list_output=$(gcloud compute instances list --project="${project}" --zones="${zone}" \
		--filter="name ~ ^${prefix}" --format='value(name)'); then
		echo "ERROR: Failed to list instances from gcloud: ${instance_list_output}" >&2
	elif [[ -n "${instance_list_output}" ]]; then
		local INSTANCES_TO_DELETE_ARRAY=()
		readarray -t INSTANCES_TO_DELETE_ARRAY <<<"${instance_list_output}"
		if ! DELETE_OUTPUT=$(gcloud compute instances delete "${INSTANCES_TO_DELETE_ARRAY[@]}" \
			--project="${project}" \
			--zone="${zone}" \
			--quiet \
			--delete-disks=all 2>&1); then
			echo "ERROR IN DELETING RESOURCE: ${DELETE_OUTPUT}" >&2
		fi
	fi
}

if ! GCS_CONTENT=$(gcloud storage cat "${OPTIONS_GCS_PATH}"); then
	echo "ERROR: Failed to read ${OPTIONS_GCS_PATH}." >&2
	exit 1
fi

declare -a ZONES_ARRAY=()
while IFS= read -r line; do
	if [[ -n "${line}" ]]; then
		ZONES_ARRAY+=("${line}")
	fi
done <<<"${GCS_CONTENT}"

if [[ "${#ZONES_ARRAY[@]}" -eq 0 ]]; then
	echo "ERROR: No valid zones found in ${OPTIONS_GCS_PATH}" >&2
	exit 1
fi

SELECTED_ZONE=""
SUCCESS=false

# Loop through all zones to find capacity
for ZONE in "${ZONES_ARRAY[@]}"; do
	readarray -t INSTANCE_NAMES_ARRAY < <(generate_instance_names "${FULL_INSTANCE_PREFIX}" "${NUM_NODES}")
	# Join instance names with commas for the --predefined-names flag
	instance_names_str=$(
		IFS=,
		echo "${INSTANCE_NAMES_ARRAY[*]}"
	)

	declare -a GCLOUD_CMD
	GCLOUD_CMD=(
		gcloud compute instances bulk create
		--predefined-names="${instance_names_str}"
		--project="${PROJECT_ID}"
		--zone="${ZONE}"
		--machine-type="${MACHINE_TYPE}"
		--provisioning-model="${PROVISIONING_MODEL}"
		--instance-termination-action="${TERMINATION_ACTION}"
		--no-address
		--quiet
		--min-count="${MIN_NODES}"
	)
	if CREATE_OUTPUT=$("${GCLOUD_CMD[@]}" 2>&1); then
		instance_list_output=$(gcloud compute instances list --project="${PROJECT_ID}" --zones="${ZONE}" \
			--filter="name ~ ^${FULL_INSTANCE_PREFIX}" --format='value(name)')

		if [[ $? -eq 0 && -n "${instance_list_output}" ]]; then
			readarray -t created_instances < <(echo "${instance_list_output}")
			NUM_CREATED=$((${#created_instances[@]}))
			echo "${ZONE} ${NUM_CREATED}"
			cleanup_instances "${PROJECT_ID}" "${ZONE}" "${FULL_INSTANCE_PREFIX}"

			if [[ "${NUM_CREATED}" -ge "${MIN_NODES}" ]]; then
				SELECTED_ZONE="${ZONE}"
				SUCCESS=true
				break
			else
				echo "ERROR: Bulk create succeeded but only ${NUM_CREATED} instances found, less than min_count ${MIN_NODES} in ${ZONE}." >&2
			fi
		else
			echo "ERROR: Bulk create command succeeded in ${ZONE}, but failed to list the created instances or none found." >&2
			cleanup_instances "${PROJECT_ID}" "${ZONE}" "${FULL_INSTANCE_PREFIX}"
		fi
	else
		# Command failed
		if [[ "${CREATE_OUTPUT}" != *"INSUFFICIENT_CAPACITY"* &&
			"${CREATE_OUTPUT}" != *"ZONE_RESOURCE_POOL_EXHAUSTED"* ]]; then
			echo "ERROR: Unexpected error during bulk create in ${ZONE}: ${CREATE_OUTPUT}" >&2
		fi
		# bulk create with --min-count should handle rollback on failure, but cleanup just in case.
		cleanup_instances "${PROJECT_ID}" "${ZONE}" "${FULL_INSTANCE_PREFIX}"
	fi
done

if [[ "${SUCCESS}" == "true" ]]; then
	echo "Deploying in ZONE: ${SELECTED_ZONE}"
	export ZONE="${SELECTED_ZONE}"
else
	echo "--- DEPLOYMENT FAILED(Couldn't find a zone to deploy) ---" >&2
	exit 1
fi
