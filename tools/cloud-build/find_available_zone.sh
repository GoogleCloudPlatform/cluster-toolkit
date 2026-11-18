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

REQUESTED_PROVISIONING_MODEL="${PROVISIONING_MODEL:-}"
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

check_single_vm_capacity() {
	local target_zone=$1
	local model=$2
	local vm_type=$3
	local node_count=$4
	local min_count=$5
	local name_prefix=$6
	local accel_spec=${7:-}
	local success=false
	local create_output=""
	local instance_list_output=""

	local -a instance_names_array=()
	readarray -t instance_names_array < <(generate_instance_names "${name_prefix}" "${node_count}")
	local instance_names_str
	instance_names_str=$(
		IFS=,
		echo "${instance_names_array[*]}"
	)

	local gcloud_cmd=(
		gcloud compute instances bulk create
		--predefined-names="${instance_names_str}"
		--project="${PROJECT_ID}"
		--zone="${target_zone}"
		--machine-type="${vm_type}"
		--provisioning-model="${model}"
		--no-address
		--quiet
		--min-count="${min_count}"
	)

	if [[ -n "${accel_spec}" ]]; then
		gcloud_cmd+=(--accelerator="${accel_spec}")
	fi

	if [[ "${model}" == "SPOT" ]]; then
		gcloud_cmd+=(--instance-termination-action="${TERMINATION_ACTION}")
		if [[ -n "${accel_spec}" ]]; then
			gcloud_cmd+=(--on-host-maintenance="TERMINATE")
		fi
	else
		gcloud_cmd+=(--on-host-maintenance="TERMINATE")
	fi

	echo "INFO: Attempting to bulk create ${node_count} ${vm_type} VMs in ${target_zone} (Model: ${model})..."
	if create_output=$("${gcloud_cmd[@]}" 2>&1); then
		if instance_list_output=$(gcloud compute instances list --project="${PROJECT_ID}" --zones="${target_zone}" \
			--filter="name ~ ^${name_prefix}" --format='value(name)'); then
			if [[ -n "${instance_list_output}" ]]; then
				local -a created_instances=()
				readarray -t created_instances < <(echo "${instance_list_output}")
				local num_created=${#created_instances[@]}
				cleanup_vm_instances "${PROJECT_ID}" "${target_zone}" "${name_prefix}"

				if [[ "${num_created}" -ge "${min_count}" ]]; then
					echo "INFO: Found sufficient ${vm_type} VM capacity in ${target_zone}."
					success=true
				else
					echo "ERROR: Bulk create & list succeeded in ${target_zone}, but only ${num_created} instances found, less than min_count ${min_count}." >&2
				fi
			else
				echo "ERROR: Bulk create command apparently succeeded in ${target_zone}, but LIST command found no instances with the prefix." >&2
				cleanup_vm_instances "${PROJECT_ID}" "${target_zone}" "${name_prefix}"
			fi
		else
			echo "ERROR: Bulk create command succeeded in ${target_zone}, but the command to list instances failed." >&2
			cleanup_vm_instances "${PROJECT_ID}" "${target_zone}" "${name_prefix}"
		fi
	else
		if [[ "${create_output}" != *"INSUFFICIENT_CAPACITY"* && "${create_output}" != *"ZONE_RESOURCE_POOL_EXHAUSTED"* ]]; then
			echo "ERROR: Unexpected error during bulk create in ${target_zone}: ${create_output}" >&2
		else
			echo "INFO: Insufficient ${vm_type} VM capacity for bulk create in ${target_zone} (Model: ${model})."
		fi
		cleanup_vm_instances "${PROJECT_ID}" "${target_zone}" "${name_prefix}"
	fi

	[[ "${success}" == "true" ]]
}

check_vm_capacity() {
	local zone=$1
	local provisioning_model=$2
	local IFS

	if ! check_single_vm_capacity "${zone}" "${provisioning_model}" "${MACHINE_TYPE}" "${NUM_NODES}" "${MIN_NODES}" "${FULL_INSTANCE_PREFIX}" "${GUEST_ACCELERATOR:-}"; then
		return 1
	fi

	if [[ -n "${ADDITIONAL_VM_CHECKS:-}" ]]; then
		local -a extra_specs=()
		IFS=';' read -r -a extra_specs <<<"${ADDITIONAL_VM_CHECKS}"
		local idx=0
		local spec
		for spec in "${extra_specs[@]}"; do
			[[ -z "${spec}" ]] && continue
			idx=$((idx + 1))
			local extra_mtype="" extra_num="" extra_min="" extra_accel=""
			IFS=':' read -r extra_mtype extra_num extra_min extra_accel <<<"${spec}"
			extra_num="${extra_num:-1}"
			extra_min="${extra_min:-${extra_num}}"
			local extra_prefix="${INSTANCE_PREFIX}x${idx}-${BUILD_ID_SHORT}-"
			if ! check_single_vm_capacity "${zone}" "${provisioning_model}" "${extra_mtype}" "${extra_num}" "${extra_min}" "${extra_prefix}" "${extra_accel}"; then
				return 1
			fi
		done
	fi

	return 0
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
		jq -r --arg region "$region" '[.[].dimensionsInfos[]? | select(((.applicableLocations? // []) | index($region)) or (.dimensions?.region? // "") == $region) | (.details?.value? // "0") | tonumber] | max' 2>/dev/null)

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

	if ! limit=$(printf "%.0f" "${limit}" 2>/dev/null); then limit=0; fi
	if ! usage=$(printf "%.0f" "${usage}" 2>/dev/null); then usage=0; fi

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
	local zone=$1
	local required_capacity=${REQUIRED_LUSTRE_GB:-0}
	if [[ "$required_capacity" -le 0 ]]; then
		return 0
	fi

	if [[ -n "${LUSTRE_QUOTA_CACHE[$zone]:-}" ]]; then
		return "${LUSTRE_QUOTA_CACHE[$zone]}"
	fi

	local limit
	limit=$(gcloud beta quotas info list --service=lustre.googleapis.com --project="${PROJECT_ID}" \
		--filter="quotaId=Capacity" --format="json" 2>/dev/null |
		jq -r --arg zone "$zone" '[.[].dimensionsInfos[]? | select(((.applicableLocations? // []) | index($zone)) or (.dimensions?.zone? // "") == $zone) | (.details?.value? // "0") | tonumber] | max' 2>/dev/null)

	if [[ "${limit}" == "-1" ]]; then
		LUSTRE_QUOTA_CACHE[$zone]=0
		return 0
	fi

	if [[ -z "$limit" || "$limit" == "null" ]]; then
		echo "WARN: Could not fetch Lustre quota limit for $zone. Failing-open and assuming capacity exists."
		LUSTRE_QUOTA_CACHE[$zone]=0
		return 0
	fi

	if [[ "$limit" == "0" ]]; then
		echo "INFO: Lustre quota limit for $zone is explicitly 0. Skipping zone."
		LUSTRE_QUOTA_CACHE[$zone]=1
		return 1
	fi

	local usage
	usage=$(gcloud lustre instances list --location="-" --project="${PROJECT_ID}" --format="json" 2>/dev/null |
		jq -r --arg zone "$zone" '[.[]? | select((.name | split("/")[3]) == $zone) | .capacityGib | select(. != null) | tonumber] | add // 0' 2>/dev/null)

	if ! limit=$(printf "%.0f" "${limit}" 2>/dev/null); then limit=0; fi
	if ! usage=$(printf "%.0f" "${usage}" 2>/dev/null); then usage=0; fi

	if [[ ! "${limit}" =~ ^[0-9]+$ ]]; then limit=0; fi
	if [[ ! "${usage}" =~ ^[0-9]+$ ]]; then usage=0; fi

	local remaining=$((limit - usage))
	if [[ "$remaining" -ge "$required_capacity" ]]; then
		LUSTRE_QUOTA_CACHE[$zone]=0
		return 0
	else
		echo "INFO: Insufficient Lustre quota in $zone (Limit: $limit, Usage: $usage, Required: $required_capacity)."
		LUSTRE_QUOTA_CACHE[$zone]=1
		return 1
	fi
}
declare -A REGION_QUOTAS_JSON_CACHE
declare -A CPU_QUOTA_CACHE
check_cpu_quota() {
	local region=$1
	local metric=${2:-${CPU_QUOTA_METRIC:-"N2D_CPUS"}}
	local required_cpus=${3:-${REQUIRED_CPU_QUOTA:-0}}
	if [[ ! "${required_cpus}" =~ ^[0-9]+$ ]]; then
		required_cpus=0
	fi
	if [[ "${required_cpus}" -le 0 ]]; then
		return 0
	fi

	local cache_key="${region}:${metric}:${required_cpus}"
	if [[ -n "${CPU_QUOTA_CACHE[${cache_key}]:-}" ]]; then
		return "${CPU_QUOTA_CACHE[${cache_key}]}"
	fi

	if [[ -z "${REGION_QUOTAS_JSON_CACHE[${region}]+x}" ]]; then
		REGION_QUOTAS_JSON_CACHE["${region}"]=$(gcloud compute regions describe "${region}" \
			--project="${PROJECT_ID}" --format="json(quotas)" 2>/dev/null || true)
	fi

	local quota_info limit usage
	quota_info=$(jq -r --arg m "${metric}" '.quotas[]? | select(.metric == $m) | "\(.limit | if . >= 1e15 then -1 else floor end) \((.usage // 0) | floor)"' <<<"${REGION_QUOTAS_JSON_CACHE[${region}]}" 2>/dev/null || true)
	read -r limit usage <<<"${quota_info}"

	if [[ ! "${limit}" =~ ^-?[0-9]+$ ]]; then
		echo "WARN: Could not fetch ${metric} quota for ${region}. Failing-open and assuming capacity exists."
		CPU_QUOTA_CACHE["${cache_key}"]=0
		return 0
	fi

	if [[ "${limit}" -lt 0 ]]; then
		CPU_QUOTA_CACHE["${cache_key}"]=0
		return 0
	fi

	if [[ ! "${usage}" =~ ^[0-9]+$ ]]; then usage=0; fi

	local remaining=$((limit - usage))
	if [[ "${remaining}" -ge "${required_cpus}" ]]; then
		CPU_QUOTA_CACHE["${cache_key}"]=0
		return 0
	else
		echo "INFO: Insufficient ${metric} quota in ${region} (Limit: ${limit}, Usage: ${usage}, Required: ${required_cpus})."
		CPU_QUOTA_CACHE["${cache_key}"]=1
		return 1
	fi
}

discover_candidate_zones() {
	local cache_file="${DISCOVERED_ZONES_CACHE_FILE:-/tmp/discovered_zones_${INSTANCE_PREFIX:-ctk}_${BUILD_ID_SHORT:-default}.txt}"
	if [[ -s "${cache_file}" ]]; then
		cat "${cache_file}"
		return 0
	fi

	declare -A candidate_map=()
	declare -A current_zones=()
	declare -A accel_zones=()
	declare -A emitted=()

	local req_machines_str="${REQUIRED_MACHINE_TYPES//,/ }"
	local req_accels_str="${REQUIRED_ACCELERATOR_TYPES:-}"
	req_accels_str="${req_accels_str//,/ }"

	local -a req_machines=()
	read -r -a req_machines <<<"${req_machines_str}"
	if [[ "${#req_machines[@]}" -eq 0 ]]; then
		echo "ERROR: REQUIRED_MACHINE_TYPES is empty during dynamic zone discovery." >&2
		return 1
	fi

	echo "INFO: Dynamically discovering zones supporting machine types [${req_machines_str}]${req_accels_str:+ and accelerators [${req_accels_str}]}..." >&2

	local initialized=false
	local mtype z_out z_line z_name

	for mtype in "${req_machines[@]}"; do
		[[ -z "${mtype}" ]] && continue
		if ! z_out=$(gcloud compute machine-types list --project="${PROJECT_ID}" \
			--filter="name=${mtype}" --format="value(zone)" 2>&1); then
			echo "ERROR: Failed to list zones for machine type ${mtype}: ${z_out}" >&2
			return 1
		fi
		current_zones=()
		while IFS= read -r z_line || [[ -n "${z_line}" ]]; do
			z_line="${z_line//$'\r'/}"
			[[ -z "${z_line}" ]] && continue
			z_name="${z_line##*/}"
			current_zones["${z_name}"]=1
		done <<<"${z_out}"

		if [[ "${initialized}" == "false" ]]; then
			for z_name in "${!current_zones[@]}"; do
				candidate_map["${z_name}"]=1
			done
			initialized=true
		else
			for z_name in "${!candidate_map[@]}"; do
				if [[ -z "${current_zones[${z_name}]:-}" ]]; then
					unset "candidate_map[${z_name}]"
				fi
			done
		fi
	done

	if [[ -n "${req_accels_str}" ]]; then
		local -a req_accels=()
		read -r -a req_accels <<<"${req_accels_str}"
		local accel
		for accel in "${req_accels[@]}"; do
			[[ -z "${accel}" ]] && continue
			if ! z_out=$(gcloud compute accelerator-types list --project="${PROJECT_ID}" \
				--filter="name=${accel}" --format="value(zone)" 2>&1); then
				echo "ERROR: Failed to list zones for accelerator type ${accel}: ${z_out}" >&2
				return 1
			fi
			accel_zones=()
			while IFS= read -r z_line || [[ -n "${z_line}" ]]; do
				z_line="${z_line//$'\r'/}"
				[[ -z "${z_line}" ]] && continue
				z_name="${z_line##*/}"
				accel_zones["${z_name}"]=1
			done <<<"${z_out}"

			for z_name in "${!candidate_map[@]}"; do
				if [[ -z "${accel_zones[${z_name}]:-}" ]]; then
					unset "candidate_map[${z_name}]"
				fi
			done
		done
	fi

	if [[ "${#candidate_map[@]}" -eq 0 ]]; then
		echo "ERROR: No zones support all required hardware (machines: ${req_machines_str}, accelerators: ${req_accels_str})." >&2
		return 1
	fi

	local -a sorted_all=()
	readarray -t sorted_all < <(printf '%s\n' "${!candidate_map[@]}" | sort)

	local pref_regions_str="${PREFERRED_REGIONS:-us-central1 us-east4 us-west1 us-east1 us-west4 europe-west4 europe-west1 asia-southeast1}"
	pref_regions_str="${pref_regions_str//,/ }"
	local -a pref_regions=()
	read -r -a pref_regions <<<"${pref_regions_str}"

	local -a ordered_zones=()
	local reg z
	for reg in "${pref_regions[@]}"; do
		[[ -z "${reg}" ]] && continue
		for z in "${sorted_all[@]}"; do
			if [[ "${z%-*}" == "${reg}" && -z "${emitted[${z}]:-}" ]]; then
				ordered_zones+=("${z}")
				emitted["${z}"]=1
			fi
		done
	done
	for z in "${sorted_all[@]}"; do
		if [[ -z "${emitted[${z}]:-}" ]]; then
			ordered_zones+=("${z}")
			emitted["${z}"]=1
		fi
	done

	echo "INFO: Discovered ${#ordered_zones[@]} candidate zones: ${ordered_zones[*]}" >&2
	printf '%s\n' "${ordered_zones[@]}" | tee "${cache_file}"
}

if [[ -n "${OPTIONS_FILE:-}" ]]; then
	OPTIONS_SOURCE="${OPTIONS_FILE}"
	if [[ ! -f "${OPTIONS_FILE}" ]] || ! GCS_CONTENT=$(cat "${OPTIONS_FILE}"); then
		echo "ERROR: Failed to read local OPTIONS_FILE ${OPTIONS_FILE}." >&2
		exit 1
	fi
elif [[ -n "${OPTIONS_GCS_PATH:-}" ]]; then
	OPTIONS_SOURCE="${OPTIONS_GCS_PATH}"
	if ! GCS_CONTENT=$(gcloud storage cat "${OPTIONS_GCS_PATH}"); then
		echo "ERROR: Failed to read ${OPTIONS_GCS_PATH}." >&2
		exit 1
	fi
elif [[ -n "${REQUIRED_MACHINE_TYPES:-}" ]]; then
	OPTIONS_SOURCE="REQUIRED_MACHINE_TYPES(${REQUIRED_MACHINE_TYPES})"
	if ! GCS_CONTENT=$(discover_candidate_zones); then
		echo "ERROR: Failed to dynamically discover candidate zones." >&2
		exit 1
	fi
else
	echo "ERROR: One of OPTIONS_FILE, OPTIONS_GCS_PATH, or REQUIRED_MACHINE_TYPES must be set." >&2
	exit 1
fi

declare -a ZONES_ARRAY=()
while IFS= read -r line || [[ -n "${line}" ]]; do
	line="${line//$'\r'/}"
	if [[ -n "${line}" ]]; then
		ZONES_ARRAY+=("${line}")
	fi
done <<<"${GCS_CONTENT}"
if [[ "${#ZONES_ARRAY[@]}" -eq 0 ]]; then
	echo "ERROR: No valid zones found in ${OPTIONS_SOURCE}" >&2
	exit 1
fi

SELECTED_ZONE=""
SUCCESS=false

declare -a PROVISIONING_MODELS=()
if [[ "${REQUESTED_PROVISIONING_MODEL}" == "STANDARD" ]]; then
	PROVISIONING_MODELS=("STANDARD")
elif [[ "${REQUESTED_PROVISIONING_MODEL}" == "SPOT" ]]; then
	PROVISIONING_MODELS=("SPOT")
else
	PROVISIONING_MODELS=("SPOT")
	if [[ "${ENABLE_SPOT_FALLBACK:-false}" == "true" ]]; then
		PROVISIONING_MODELS+=("STANDARD")
	fi
fi

FILESTORE_ZONES=""
if [[ "${CHECK_FILESTORE:-false}" == "true" ]]; then
	echo "INFO: Fetching available Filestore zones..."
	if ! FILESTORE_ZONES=$(gcloud filestore locations list --project="${PROJECT_ID}" --format="value(locationId)" 2>&1); then
		echo "ERROR: Failed to fetch available Filestore locations: ${FILESTORE_ZONES}" >&2
		exit 1
	fi
fi

LUSTRE_ZONES=""
if [[ "${CHECK_LUSTRE:-false}" == "true" ]]; then
	echo "INFO: Fetching available Managed Lustre zones..."
	if ! LUSTRE_ZONES=$(gcloud lustre locations list --project="${PROJECT_ID}" --format="value(locationId)" 2>&1); then
		echo "ERROR: Failed to fetch available Lustre locations: ${LUSTRE_ZONES}" >&2
		exit 1
	fi
fi

for PROVISIONING_MODEL in "${PROVISIONING_MODELS[@]}"; do
	echo "INFO: Trying provisioning model: ${PROVISIONING_MODEL}"

	for ZONE in "${ZONES_ARRAY[@]}"; do
		REGION=${ZONE%-*}

		# Check for explicit zone exclusion (e.g. from retry harness or manual override)
		if [[ -n "${EXCLUDE_ZONES:-}" ]]; then
			if [[ " ${EXCLUDE_ZONES//,/ } " == *" ${ZONE} "* ]]; then
				echo "INFO: Skipping ${ZONE} - Zone explicitly excluded via EXCLUDE_ZONES."
				continue
			fi
		fi

		if [[ "${CHECK_FILESTORE:-false}" == "true" ]]; then
			if ! echo "${FILESTORE_ZONES}" | grep -x -E -q "${ZONE}|${REGION}"; then
				echo "INFO: Skipping ${ZONE} - Filestore not available in this zone or region."
				continue
			fi
			if ! check_filestore_quota "${REGION}"; then
				echo "INFO: Skipping ${ZONE} - Filestore quota check failed in region ${REGION}."
				continue
			fi
		fi

		if [[ "${CHECK_LUSTRE:-false}" == "true" ]]; then
			if [[ -n "${LUSTRE_EXCLUDE_ZONES:-}" && "${LUSTRE_EXCLUDE_ZONES}" != "none" ]]; then
				if [[ " ${LUSTRE_EXCLUDE_ZONES//,/ } " == *" ${ZONE} "* ]]; then
					echo "INFO: Skipping ${ZONE} - Zone excluded due to known Managed Lustre capacity exhaustion."
					continue
				fi
			fi

			if ! echo "${LUSTRE_ZONES}" | grep -x -q "${ZONE}"; then
				echo "INFO: Skipping ${ZONE} - Managed Lustre not available in this zone."
				continue
			fi
			if ! check_lustre_quota "${ZONE}"; then
				echo "INFO: Skipping ${ZONE} - Lustre quota check failed in zone ${ZONE}."
				continue
			fi
		fi

		# Check region for support VMs.
		if ! check_cpu_quota "${REGION}"; then
			echo "INFO: Skipping ${ZONE} - CPU quota check failed in region ${REGION}."
			continue
		fi

		if [[ -n "${ADDITIONAL_CPU_QUOTAS:-}" ]]; then
			EXTRA_QUOTA_OK=true
			EXTRA_QUOTAS_STR="${ADDITIONAL_CPU_QUOTAS//,/ }"
			read -r -a EXTRA_QUOTA_SPECS <<<"${EXTRA_QUOTAS_STR}"
			for quota_spec in "${EXTRA_QUOTA_SPECS[@]}"; do
				[[ -z "${quota_spec}" ]] && continue
				if [[ "${quota_spec}" != *:* ]]; then
					echo "WARN: Malformed quota spec '${quota_spec}' in ADDITIONAL_CPU_QUOTAS (expected METRIC:COUNT); skipping." >&2
					continue
				fi
				extra_q_metric="${quota_spec%%:*}"
				extra_q_req="${quota_spec#*:}"
				if ! check_cpu_quota "${REGION}" "${extra_q_metric}" "${extra_q_req}"; then
					echo "INFO: Skipping ${ZONE} - Additional CPU quota check (${extra_q_metric}) failed in region ${REGION}."
					EXTRA_QUOTA_OK=false
					break
				fi
			done
			if [[ "${EXTRA_QUOTA_OK}" == "false" ]]; then
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
