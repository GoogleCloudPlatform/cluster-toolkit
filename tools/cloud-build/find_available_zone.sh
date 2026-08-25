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

BUILD_ID_SHORT="${BUILD_ID:0:6}"
PROVISIONING_MODEL="SPOT"
TERMINATION_ACTION="DELETE"
FULL_INSTANCE_PREFIX="${INSTANCE_PREFIX}-${BUILD_ID_SHORT}-"
MIN_NODES="${MIN_NODES:-2}" # Define minimum number of nodes required

generate_instance_names() {
	local prefix=$1
	local num=$2
	for i in $(seq -w 01 "$num"); do
		echo "${prefix}${i}"
	done
}

cleanup_vm_instances() {
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

cleanup_tpu_nodes() {
	local project=$1
	local zone=$2
	# Shift out project and zone
	shift 2
	local tpu_names=("$@")

	if [[ "${#tpu_names[@]}" -eq 0 ]]; then
		return
	fi

	for tpu_name in "${tpu_names[@]}"; do
		if ! DELETE_OUTPUT=$(gcloud compute tpus tpu-vm delete "${tpu_name}" --project="${project}" --zone="${zone}" --quiet 2>&1); then
			echo "ERROR IN DELETING TPU RESOURCE ${tpu_name}: ${DELETE_OUTPUT}" >&2
		fi
	done
}

check_tpu_capacity() {
	local zone=$1
	local provisioning_model=$2
	local success=false

	# --- TPU Capacity Check ---
	for i in $(seq 1 "${NUM_NODES}"); do
		local tpu_name
		tpu_name="${FULL_INSTANCE_PREFIX}$(printf "%02d" "$i")"
		local gcloud_tpu_cmd=(
			gcloud compute tpus tpu-vm create "${tpu_name}"
			--project="${PROJECT_ID}"
			--zone="${zone}"
			--accelerator-type="${ACCELERATOR_TYPE}"
			--version="${TPU_RUNTIME_VERSION}"
			--quiet
		)
		if [[ "${provisioning_model}" == "SPOT" ]]; then
			gcloud_tpu_cmd+=(--spot)
		fi

		if ! tpu_create_output=$("${gcloud_tpu_cmd[@]}" 2>&1); then
			echo "ERROR: Unexpected error during TPU create in ${zone} (${provisioning_model}): ${tpu_create_output}" >&2
		fi
	done

	local created_tpu_names=()
	local tpu_created_count=0
	if tpu_list_output=$(gcloud compute tpus tpu-vm list --project="${PROJECT_ID}" --zone="${zone}" \
		--filter="name ~ ${FULL_INSTANCE_PREFIX}" --format='value(name)' 2>/dev/null); then
		if [[ -n "${tpu_list_output}" ]]; then
			readarray -t created_tpu_names <<<"${tpu_list_output}"
			tpu_created_count=${#created_tpu_names[@]}
			echo "INFO: Found ${tpu_created_count} TPU nodes: ${created_tpu_names[*]}"
		else
			echo "INFO: No matching TPU nodes found in ${zone} via list."
		fi
	else
		echo "ERROR: Failed to list TPU nodes in ${zone}: ${tpu_list_output}" >&2
	fi

	if [[ "${tpu_created_count}" -ge 1 ]]; then
		success=true
	fi

	cleanup_tpu_nodes "${PROJECT_ID}" "${zone}" "${created_tpu_names[@]}"
	[[ "${success}" == "true" ]]
}

check_vm_capacity() {
	local zone=$1
	local provisioning_model=$2
	local success=false

	readarray -t instance_names_array < <(generate_instance_names "${FULL_INSTANCE_PREFIX}" "${NUM_NODES}")
	local instance_names_str
	instance_names_str=$(
		IFS=,
		echo "${instance_names_array[*]}"
	)

	local gcloud_cmd=(
		gcloud compute instances bulk create
		--predefined-names="${instance_names_str}"
		--project="${PROJECT_ID}"
		--zone="${zone}"
		--machine-type="${MACHINE_TYPE}"
		--provisioning-model="${provisioning_model}"
		--no-address
		--quiet
		--min-count="${MIN_NODES}"
	)

	if [[ "${provisioning_model}" == "SPOT" ]]; then
		gcloud_cmd+=(--instance-termination-action="${TERMINATION_ACTION}")
	else
		gcloud_cmd+=(--on-host-maintenance="TERMINATE")
	fi

	echo "INFO: Attempting to bulk create ${NUM_NODES} VMs in ${zone} (Model: ${provisioning_model})..."
	if create_output=$("${gcloud_cmd[@]}" 2>&1); then
		if instance_list_output=$(gcloud compute instances list --project="${PROJECT_ID}" --zones="${zone}" \
			--filter="name ~ ^${FULL_INSTANCE_PREFIX}" --format='value(name)'); then
			if [[ -n "${instance_list_output}" ]]; then
				readarray -t created_instances < <(echo "${instance_list_output}")
				local num_created=${#created_instances[@]}
				cleanup_vm_instances "${PROJECT_ID}" "${zone}" "${FULL_INSTANCE_PREFIX}"

				if [[ "${num_created}" -ge "${MIN_NODES}" ]]; then
					echo "INFO: Found sufficient VM capacity in ${zone}."
					success=true
				else
					echo "ERROR: Bulk create & list succeeded in ${zone}, but only ${num_created} instances found, less than min_count ${MIN_NODES}." >&2
				fi
			else
				echo "ERROR: Bulk create command apparently succeeded in ${zone}, but LIST command found no instances with the prefix." >&2
				cleanup_vm_instances "${PROJECT_ID}" "${zone}" "${FULL_INSTANCE_PREFIX}"
			fi
		else
			echo "ERROR: Bulk create command succeeded in ${zone}, but the command to list instances failed." >&2
			cleanup_vm_instances "${PROJECT_ID}" "${zone}" "${FULL_INSTANCE_PREFIX}"
		fi
	else
		if [[ "${create_output}" != *"INSUFFICIENT_CAPACITY"* && "${create_output}" != *"ZONE_RESOURCE_POOL_EXHAUSTED"* ]]; then
			echo "ERROR: Unexpected error during bulk create in ${zone}: ${create_output}" >&2
		else
			echo "INFO: Insufficient VM capacity for bulk create in ${zone} (Model: ${provisioning_model})."
		fi
		cleanup_vm_instances "${PROJECT_ID}" "${zone}" "${FULL_INSTANCE_PREFIX}"
	fi

	[[ "${success}" == "true" ]]
}

declare -A FILESTORE_QUOTA_CACHE
check_filestore_quota() {
	local region=$1
	local required_capacity=${REQUIRED_FILESTORE_GB:-0}
	local tier=${REQUIRED_FILESTORE_TIER:-"BASIC_SSD"} # Default to BASIC_SSD if not specified
	tier="${tier^^}"

	if [[ "$required_capacity" -le 0 ]]; then
		return 0
	fi

	if [[ -n "${FILESTORE_QUOTA_CACHE[$region]:-}" ]]; then
		return "${FILESTORE_QUOTA_CACHE[$region]}"
	fi

	local quota_id
	case "${tier}" in
	"BASIC_HDD" | "STANDARD")
		quota_id="StandardStorageGbPerRegion"
		tier="BASIC_HDD"
		;;
	"BASIC_SSD" | "PREMIUM")
		quota_id="PremiumStorageGbPerRegion"
		tier="BASIC_SSD"
		;;
	"HIGH_SCALE_SSD")
		quota_id="HighScaleSSDStorageGibPerRegion"
		;;
	"ENTERPRISE")
		quota_id="EnterpriseStorageGibPerRegion"
		;;
	*)
		echo "WARN: Unknown Filestore tier '${tier}'. Falling back to PremiumStorageGbPerRegion."
		quota_id="PremiumStorageGbPerRegion"
		tier="BASIC_SSD"
		;;
	esac

	local limit
	limit=$(gcloud beta quotas info list --service=file.googleapis.com --project="${PROJECT_ID}" \
		--filter="quotaId=${quota_id}" --format="json" 2>/dev/null |
		jq -r --arg region "$region" '[.[].dimensionsInfos[]? | select((.applicableLocations? // []) | index($region) or (.dimensions?.region? // "") == $region) | .details?.value? | select(. != null) | tonumber] | max' 2>/dev/null)

	if [[ "${limit}" == "-1" ]]; then
		FILESTORE_QUOTA_CACHE[$region]=0
		return 0
	fi

	if [[ -z "$limit" || "$limit" == "null" ]]; then
		echo "WARN: Could not fetch Filestore quota limit (${quota_id}) for $region. Failing-open and assuming capacity exists."
		FILESTORE_QUOTA_CACHE[$region]=0
		return 0
	fi

	if [[ "$limit" == "0" ]]; then
		echo "INFO: Filestore quota limit (${quota_id}) for $region is explicitly 0. Skipping zone."
		FILESTORE_QUOTA_CACHE[$region]=1
		return 1
	fi

	local usage
	usage=$(gcloud filestore instances list --project="${PROJECT_ID}" --format="json" 2>/dev/null |
		jq -r --arg region "$region" --arg tier "$tier" '[.[]? | select((.name | split("/")[3]) as $loc | $loc == $region or ($loc | startswith($region + "-"))) | select(.tier == $tier) | .fileShares[]?.capacityGb | select(. != null) | tonumber] | add // 0' 2>/dev/null)

	limit=$(printf "%.0f" "${limit}" 2>/dev/null || echo 0)
	usage=$(printf "%.0f" "${usage}" 2>/dev/null || echo 0)

	if [[ ! "${limit}" =~ ^[0-9]+$ ]]; then limit=0; fi
	if [[ ! "${usage}" =~ ^[0-9]+$ ]]; then usage=0; fi

	local remaining=$((limit - usage))
	if [[ "$remaining" -ge "$required_capacity" ]]; then
		FILESTORE_QUOTA_CACHE[$region]=0
		return 0
	else
		echo "INFO: Insufficient Filestore quota (${tier}/${quota_id}) in $region (Limit: $limit, Usage: $usage, Required: $required_capacity)."
		FILESTORE_QUOTA_CACHE[$region]=1
		return 1
	fi
}

declare -A LUSTRE_QUOTA_CACHE
check_lustre_quota() {
	local region=$1
	local required_capacity=${REQUIRED_LUSTRE_GB:-0}
	if [[ "$required_capacity" -le 0 ]]; then
		return 0
	fi

	if [[ -n "${LUSTRE_QUOTA_CACHE[$region]:-}" ]]; then
		return "${LUSTRE_QUOTA_CACHE[$region]}"
	fi

	local limit
	limit=$(gcloud beta quotas info list --service=parallelstore.googleapis.com --project="${PROJECT_ID}" \
		--filter="quotaId=StorageGiBPerRegion" --format="json" 2>/dev/null |
		jq -r --arg region "$region" '[.[].dimensionsInfos[]? | select((.applicableLocations? // []) | index($region) or (.dimensions?.region? // "") == $region) | .details?.value? | select(. != null) | tonumber] | max' 2>/dev/null)

	if [[ "${limit}" == "-1" ]]; then
		LUSTRE_QUOTA_CACHE[$region]=0
		return 0
	fi

	if [[ -z "$limit" || "$limit" == "null" ]]; then
		echo "WARN: Could not fetch Lustre quota limit for $region. Failing-open and assuming capacity exists."
		LUSTRE_QUOTA_CACHE[$region]=0
		return 0
	fi

	if [[ "$limit" == "0" ]]; then
		echo "INFO: Lustre quota limit for $region is explicitly 0. Skipping zone."
		LUSTRE_QUOTA_CACHE[$region]=1
		return 1
	fi

	local usage
	usage=$(gcloud parallelstore instances list --location="-" --project="${PROJECT_ID}" --format="json" 2>/dev/null |
		jq -r --arg region "$region" '[.[]? | select((.name | split("/")[3]) as $loc | $loc == $region or ($loc | startswith($region + "-"))) | .capacityGib | select(. != null) | tonumber] | add // 0' 2>/dev/null)

	limit=$(printf "%.0f" "${limit}" 2>/dev/null || echo 0)
	usage=$(printf "%.0f" "${usage}" 2>/dev/null || echo 0)

	if [[ ! "${limit}" =~ ^[0-9]+$ ]]; then limit=0; fi
	if [[ ! "${usage}" =~ ^[0-9]+$ ]]; then usage=0; fi

	local remaining=$((limit - usage))
	if [[ "$remaining" -ge "$required_capacity" ]]; then
		LUSTRE_QUOTA_CACHE[$region]=0
		return 0
	else
		echo "INFO: Insufficient Lustre quota in $region (Limit: $limit, Usage: $usage, Required: $required_capacity)."
		LUSTRE_QUOTA_CACHE[$region]=1
		return 1
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

declare -a PROVISIONING_MODELS=("SPOT")
if [[ "${ENABLE_SPOT_FALLBACK:-false}" == "true" ]]; then
	PROVISIONING_MODELS+=("STANDARD")
fi

FILESTORE_ZONES=""
if [[ "${CHECK_FILESTORE}" == "true" ]]; then
	echo "INFO: Fetching available Filestore zones..."
	if ! FILESTORE_ZONES=$(gcloud filestore locations list --project="${PROJECT_ID}" --format="value(locationId)" 2>&1); then
		echo "ERROR: Failed to fetch available Filestore locations: ${FILESTORE_ZONES}" >&2
		exit 1
	fi
fi

LUSTRE_ZONES=""
if [[ "${CHECK_LUSTRE}" == "true" ]]; then
	echo "INFO: Fetching available Lustre (Parallelstore) zones..."
	if ! LUSTRE_ZONES=$(gcloud parallelstore locations list --project="${PROJECT_ID}" --format="value(locationId)" 2>&1); then
		echo "ERROR: Failed to fetch available Lustre locations: ${LUSTRE_ZONES}" >&2
		exit 1
	fi
fi

for PROVISIONING_MODEL in "${PROVISIONING_MODELS[@]}"; do
	echo "INFO: Trying provisioning model: ${PROVISIONING_MODEL}"

	for ZONE in "${ZONES_ARRAY[@]}"; do
		REGION=${ZONE%-*}

		if [[ "${CHECK_FILESTORE}" == "true" ]]; then
			if ! echo "${FILESTORE_ZONES}" | grep -x -E -q "${ZONE}|${REGION}"; then
				echo "INFO: Skipping ${ZONE} - Filestore not available in this zone or region."
				continue
			fi
			if ! check_filestore_quota "${REGION}"; then
				echo "INFO: Skipping ${ZONE} - Filestore quota check failed in region ${REGION}."
				continue
			fi
		fi

		if [[ "${CHECK_LUSTRE}" == "true" ]]; then
			if ! echo "${LUSTRE_ZONES}" | grep -x -q "${ZONE}"; then
				echo "INFO: Skipping ${ZONE} - Lustre (Parallelstore) not available in this zone."
				continue
			fi
			if ! check_lustre_quota "${REGION}"; then
				echo "INFO: Skipping ${ZONE} - Lustre quota check failed in region ${REGION}."
				continue
			fi
		fi

		if [[ "${MACHINE_TYPE}" == "tpu" ]]; then
			if check_tpu_capacity "${ZONE}" "${PROVISIONING_MODEL}"; then
				SELECTED_ZONE="${ZONE}"
				SUCCESS=true
				break
			fi
		else
			if check_vm_capacity "${ZONE}" "${PROVISIONING_MODEL}"; then
				SELECTED_ZONE="${ZONE}"
				SUCCESS=true
				break
			fi
		fi
	done

	if [[ "${SUCCESS}" == "true" ]]; then
		break
	fi
done

if [[ "${SUCCESS}" == "true" ]]; then
	echo "Deploying in ZONE: ${SELECTED_ZONE}, MODEL: ${PROVISIONING_MODEL}"
	export ZONE="${SELECTED_ZONE}"
	export PROVISIONING_MODEL="${PROVISIONING_MODEL}"
else
	echo "--- DEPLOYMENT FAILED(Couldn't find a zone to deploy) ---" >&2
	exit 1
fi
