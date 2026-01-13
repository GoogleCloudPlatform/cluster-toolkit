#!/bin/bash
# Copyright 2025 Google LLC
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

# CONFIGURATION & GLOBAL VARIABLES

# Associative array for resource names to exclude from deletion
declare -A EXCLUSION_MAP
ERROR_COUNT=0

# To store IPs of protected instances, used to find and protect matching Address resources
declare -A PROTECTED_IPS
# To store Network URIs used by protected instances, used to protect related resources like Filestore
declare -A PROTECTED_NETWORK_URIS

# HELPER FUNCTIONS

log() {
	local level="$1"
	local message="$2"
	echo "[$(date +'%Y-%m-%d %H:%M:%S')] [$level] $message"
}

check_dependencies() {
	log "INFO" "Checking for required command-line tools..."
	local dependencies=("gcloud" "awk" "grep" "sort" "date" "sed" "basename")
	local missing_deps=()

	for cmd in "${dependencies[@]}"; do
		if ! command -v "$cmd" &>/dev/null; then
			missing_deps+=("$cmd")
		fi
	done

	if [ ${#missing_deps[@]} -ne 0 ]; then
		log "ERROR" "Missing required dependencies: ${missing_deps[*]}. Please install them and try again."
		exit 1 # Dependencies are critical, we must exit after reporting all missing ones.
	fi
	log "INFO" "All dependencies are satisfied."
}

load_exclusions() {
	log "INFO" "Loading exclusion list from $EXCLUSION_FILE..."

	local line_count=0
	# Helper function to process each line from the exclusion source
	process_line() {
		local line="$1"
		local trimmed_line
		trimmed_line=$(echo "$line" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
		if [[ -n "$trimmed_line" ]] && [[ "$trimmed_line" != \#* ]] && [[ -z "${EXCLUSION_MAP[${trimmed_line}]:-}" ]]; then
			EXCLUSION_MAP["${trimmed_line}"]=1
			((line_count++))
		fi
	}

	log "INFO" "Exclusion file must be a GCS object. Checking if it exists and is accessible"
	# Preliminary check to see if the GCS object (exclusion file) exists and is accessible
	if ! gcloud storage ls "$EXCLUSION_FILE" >/dev/null 2>&1; then
		log "ERROR" "Cannot access GCS exclusion file: $EXCLUSION_FILE. Please check the path and permissions."
		exit 1
	fi
	log "INFO" "Loading exclusions from GCS path: $EXCLUSION_FILE"
	# Read the exclusion file line by line from GCS
	while IFS= read -r line || [[ -n "$line" ]]; do
		process_line "$line"
	done < <(gcloud storage cat "$EXCLUSION_FILE")
	# As we make complete transition from exclusion list to use of labels, this part can be removed.
	# Currently keeping it here to prevent deletion of important resources.
	if [[ ${#EXCLUSION_MAP[@]} -eq 0 ]]; then
		log "ERROR" "No valid exclusion entries were loaded from $EXCLUSION_FILE. Exiting to prevent accidental deletion of resources."
		exit 1
	else
		log "INFO" "Loaded ${#EXCLUSION_MAP[@]} unique resource names to exclude from deletion."
	fi
}

# Checks if a resource should be excluded from deletion.
# Returns 0 if EXCLUDED (DO NOT delete)
# Returns 1 if NOT excluded (OK to delete)
is_excluded() {
	local resource_name="$1"
	local labels_str="${2:-}" # Expected format: key1=value1;key2=value2

	# Check if the resource name is in the exclusion map
	if [[ -n "${EXCLUSION_MAP[${resource_name}]:-}" ]]; then
		log "SKIP" "$resource_name is in the exclusion map."
		return 0 # Excluded
	fi

	# Check for cleanup-exemption-date label
	if [[ -n "$labels_str" ]]; then
		IFS=';' read -ra LABEL_PAIRS <<<"$labels_str"
		for PAIR in "${LABEL_PAIRS[@]}"; do
			local KEY VAL
			KEY="${PAIR%%=*}"
			VAL="${PAIR#*=}"
			if [[ "$KEY" == "cleanup-exemption-date" ]]; then
				local exp_seconds
				if [[ "$VAL" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] &&
					exp_seconds=$(date -d "$VAL + 1 day" -u +%s 2>/dev/null); then
					# Format is valid AND date parsing succeeded
					local current_seconds
					current_seconds=$(date -u +%s)
					if [[ "$exp_seconds" -gt "$current_seconds" ]]; then
						log "SKIP" "$resource_name: Exempted by active label: cleanup-exemption-date=$VAL"
						return 0 # Excluded
					else
						log "INFO" "$resource_name: Exemption label cleanup-exemption-date=$VAL has expired."
						return 1 # Not excluded because exemption expired
					fi
				else
					# Either format is invalid OR date parsing failed
					log "WARNING" "$resource_name: Invalid format or value for label cleanup-exemption-date: $VAL. Expected YYYY-MM-DD."
					return 1 # Not excluded
				fi
			fi
		done
	fi
	return 1 # Not excluded
}

# Executes the delete command if not in DRY_RUN mode.
execute_delete() {
	local resource_type="$1"
	local resource_name="$2"
	local extra_info="$3"
	shift 3
	local -a cmd_to_run=("$@")

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN" "Would delete $resource_type: $resource_name $extra_info"
		log "DRY-RUN" "Command: ${cmd_to_run[*]}"
	else
		log "EXECUTE" "Deleting $resource_type: $resource_name $extra_info"
		if "${cmd_to_run[@]}"; then
			log "SUCCESS" "Successfully deleted $resource_type: $resource_name"
		else
			log "ERROR" "Failed to delete $resource_type: $resource_name"
			((ERROR_COUNT++)) || true # Prevent exit if set -e is active
		fi
	fi
}

# Helper function to add network and subnetwork names to the EXCLUSION_MAP
# and network URIs to PROTECTED_NETWORK_URIS.
_protect_network_resources() {
	local source_resource_type="$1"
	local source_resource_name="$2"
	local net_url="$3"
	local sub_url="$4"
	local net_name
	local sub_name

	# Protect Network
	if [[ -n "$net_url" && "$net_url" != "None" ]]; then
		net_name=$(basename "${net_url}")
		if [[ -n "${net_name}" && -z "${EXCLUSION_MAP[${net_name}]:-}" ]]; then
			log "INFO" "Protecting Network '${net_name}' (used by ${source_resource_type} ${source_resource_name}) by adding to exclusion map."
			EXCLUSION_MAP["${net_name}"]=1
		fi
		# Store the full network URI for Filestore and Instance Template matching
		log "DEBUG" "Adding protected network URI (for ${source_resource_type} ${source_resource_name}): ${net_url}"
		PROTECTED_NETWORK_URIS["${net_url}"]=1
	fi

	# Protect Subnetwork
	if [[ -n "$sub_url" && "$sub_url" != "None" ]]; then
		sub_name=$(basename "${sub_url}")
		if [[ -n "${sub_name}" && -z "${EXCLUSION_MAP[${sub_name}]:-}" ]]; then
			log "INFO" "Protecting Subnetwork '${sub_name}' (used by ${source_resource_type} ${source_resource_name}) by adding to exclusion map."
			EXCLUSION_MAP["${sub_name}"]=1
		fi
	fi
}

# Populates EXCLUSION_MAP and other protection arrays based on resources
# that should not be deleted (e.g., resources in excluded GKE clusters,
# instances with exemption labels, and their dependencies).
populate_protected_resources() {
	log "INFO" "Identifying resources to protect from deletion..."
	declare -A INSTANCES_TO_PROTECT # Map instance_name -> zone
	declare -A TPUS_TO_PROTECT      # Map tpu_name -> zone

	# Part 1: Protect instances, templates, and networks used by EXCLUDED GKE clusters.
	log "INFO" "Checking for GKE clusters to protect..."
	local clusters_data
	if ! clusters_data=$(gcloud container clusters list --project="$PROJECT_ID" --format="value(name,location,resourceLabels.map())"); then
		log "ERROR" "Failed to list GKE clusters."
		((ERROR_COUNT++)) || true
	else
		while IFS=$'\t' read -r cluster_name location labels_str; do
			if [[ -z "$cluster_name" ]]; then
				log "DEBUG" "Skipping GKE cluster line with empty name."
				continue
			fi

			if ! is_excluded "$cluster_name" "$labels_str"; then # Returns 1 if NOT excluded
				continue
			fi

			log "INFO" "GKE Cluster ${cluster_name} in ${location} is PROTECTED."
			EXCLUSION_MAP["${cluster_name}"]=1 # Add cluster itself to exclusion map

			local node_pools_data
			if ! node_pools_data=$(gcloud container node-pools list --cluster="${cluster_name}" --location="${location}" --project="${PROJECT_ID}" --format="value(name)"); then
				log "WARNING" "Failed to list node pools for protected cluster ${cluster_name}."
				continue
			fi

			while IFS=$'\t' read -r np_name; do
				local ig_urls
				if ! ig_urls=$(gcloud container node-pools describe "${np_name}" --cluster="${cluster_name}" --location="${location}" --project="${PROJECT_ID}" --format="value(instanceGroupUrls)"); then
					log "WARNING" "Failed to describe node pool ${np_name} in cluster ${cluster_name}."
					continue
				fi

				IFS=';' read -ra ig_url_list <<<"$ig_urls"
				for ig_url in "${ig_url_list[@]}"; do
					local ig_name
					ig_name=$(basename "${ig_url}")
					local ig_scope_type
					ig_scope_type=$(echo "${ig_url}" | awk -F'/' '{print $(NF-3)}')
					local ig_scope_name
					ig_scope_name=$(echo "${ig_url}" | awk -F'/' '{print $(NF-2)}')
					local scope_flag=""

					if [[ "$ig_scope_type" == "zones" ]]; then
						scope_flag="--zone=${ig_scope_name}"
					elif [[ "$ig_scope_type" == "regions" ]]; then
						scope_flag="--region=${ig_scope_name}"
					else
						log "WARNING" "Unknown scope type ('${ig_scope_type}') for instance group: ${ig_url}"
						continue
					fi

					log "DEBUG" "Processing MIG: ${ig_name} (${scope_flag}) for cluster ${cluster_name}"

					# Get the instance template from the MIG
					local template_url_from_mig
					if ! template_url_from_mig=$(gcloud compute instance-groups managed describe "${ig_name}" --project="${PROJECT_ID}" "${scope_flag}" --format="value(instanceTemplate)"); then
						log "WARNING" "Failed to get instance template for MIG ${ig_name}."
					else
						log "DEBUG" "MIG ${ig_name}: instanceTemplate URL is '${template_url_from_mig}'"
						if [[ -n "$template_url_from_mig" && "$template_url_from_mig" != "None" ]]; then
							local template_name
							template_name=$(basename "$template_url_from_mig")
							log "DEBUG" "MIG ${ig_name}: Extracted template name is '${template_name}'"
							if [[ -n "$template_name" && "$template_name" != "None" ]]; then
								if ! [[ -v EXCLUSION_MAP["$template_name"] ]]; then
									log "INFO" "Excluding Instance Template (from GKE MIG ${ig_name}): ${template_name}"
									EXCLUSION_MAP["${template_name}"]=1
								else
									log "DEBUG" "Instance Template ${template_name} (from GKE MIG ${ig_name}) is already excluded."
								fi
							else
								log "WARNING" "Could not extract a valid template name from URL '${template_url_from_mig}' for MIG ${ig_name}"
							fi
						else
							log "DEBUG" "MIG ${ig_name}: No instanceTemplate URL found or value is 'None'."
						fi
					fi

					local instances_in_mig
					if ! instances_in_mig=$(gcloud compute instance-groups managed list-instances "${ig_name}" --project="${PROJECT_ID}" "${scope_flag}" --format="value(NAME,ZONE)"); then
						log "WARNING" "Failed to list instances for MIG ${ig_name}."
						continue
					fi

					while IFS=$'\t' read -r inst_name inst_zone_url; do
						if [[ -n "$inst_name" ]]; then
							local inst_zone
							inst_zone=$(basename "$inst_zone_url")
							if ! [[ -v EXCLUSION_MAP["$inst_name"] ]]; then
								log "INFO" "Protecting Instance (from GKE ${cluster_name}): ${inst_name} in ${inst_zone}"
								EXCLUSION_MAP["${inst_name}"]=1
							fi
							INSTANCES_TO_PROTECT["${inst_name}"]="${inst_zone}"
						fi
					done <<<"$instances_in_mig"
				done
			done <<<"$node_pools_data"
		done <<<"$clusters_data"
	fi

	# Part 2: Protect instances based on their name in EXCLUSION_MAP or labels.
	log "INFO" "Checking for Compute Instances to protect..."
	local instances_data
	if ! instances_data=$(gcloud compute instances list \
		--project="$PROJECT_ID" \
		--format="value(name,zone.basename(),labels.map())"); then
		log "ERROR" "Failed to list Compute Instances."
		((ERROR_COUNT++)) || true
	else
		while IFS=$'\t' read -r inst_name zone labels_str; do
			[[ -z "$inst_name" ]] && continue
			if is_excluded "$inst_name" "$labels_str"; then # Returns 0 if excluded
				if ! [[ -v EXCLUSION_MAP["$inst_name"] ]]; then
					log "INFO" "Protecting Instance: ${inst_name} in ${zone}"
					EXCLUSION_MAP["${inst_name}"]=1
				fi
				INSTANCES_TO_PROTECT["${inst_name}"]="${zone}"
			fi
		done <<<"$instances_data"
	fi

	# Part 3: Protect resources associated with the collected INSTANCES_TO_PROTECT.
	if ((${#INSTANCES_TO_PROTECT[@]} > 0)); then
		log "INFO" "Protecting sub-resources for ${#INSTANCES_TO_PROTECT[@]} protected instances..."
		for inst_name in "${!INSTANCES_TO_PROTECT[@]}"; do
			local zone="${INSTANCES_TO_PROTECT[$inst_name]}"
			log "DEBUG" "Fetching details for protected instance: ${inst_name} in ${zone}"
			local inst_details
			if ! inst_details=$(gcloud compute instances describe "${inst_name}" --zone="${zone}" --project="${PROJECT_ID}" \
				--format="value(disks[].source.list(separator=';'),networkInterfaces[].network.list(separator=';'),networkInterfaces[].subnetwork.list(separator=';'),networkInterfaces[].networkIP.list(separator=';'),networkInterfaces[].accessConfigs[].natIP.list(separator=';'),sourceInstanceTemplate)"); then
				log "WARNING" "Failed to describe protected instance ${inst_name} in ${zone}. Its sub-resources might not be protected."
				continue
			fi

			local disks_list nets_list subs_list ips_list nat_ips_list template_url
			IFS=$'\t' read -r disks_list nets_list subs_list ips_list nat_ips_list template_url <<<"$inst_details"

			# Protect Instance Template (for non-MIG instances)
			if [[ -n "$template_url" ]]; then
				local template_name
				template_name=$(basename "$template_url")
				if [[ -n "$template_name" && "$template_name" != "None" ]]; then
					if ! [[ -v EXCLUSION_MAP["$template_name"] ]]; then
						log "INFO" "Protecting Instance Template '${template_name}' (used by instance ${inst_name}) by adding to exclusion map."
						EXCLUSION_MAP["${template_name}"]=1
					else
						log "DEBUG" "Instance Template ${template_name} (for ${inst_name}) is already excluded."
					fi
				else
					log "DEBUG" "Instance ${inst_name}: No valid template name found from URL '${template_url}'"
				fi
			else
				log "DEBUG" "Instance ${inst_name}: sourceInstanceTemplate is empty or not set."
			fi

			# Protect Attached Disks
			IFS=';' read -ra disk_urls <<<"$disks_list"
			for disk_url in "${disk_urls[@]}"; do
				[[ -z "$disk_url" ]] && continue
				local disk_name
				disk_name=$(basename "${disk_url}")
				if [[ -n "${disk_name}" && -z "${EXCLUSION_MAP[${disk_name}]:-}" ]]; then
					log "INFO" "Protecting Disk '${disk_name}' (attached to instance ${inst_name}) by adding to exclusion map."
					EXCLUSION_MAP["${disk_name}"]=1
				fi
			done

			# Protect Networks & Subnetworks used by the instance
			IFS=';' read -ra net_urls <<<"$nets_list"
			IFS=';' read -ra sub_urls <<<"$subs_list"
			for i in "${!net_urls[@]}"; do
				_protect_network_resources "Instance" "$inst_name" "${net_urls[$i]:-}" "${sub_urls[$i]:-}"
			done

			# Collect all IPs (internal and external) used by the instance to protect Address resources later
			IFS=';' read -ra network_ips <<<"$ips_list"
			for ip in "${network_ips[@]}"; do [[ -n "$ip" ]] && PROTECTED_IPS["${ip}"]=1; done
			IFS=';' read -ra nat_ips <<<"$nat_ips_list"
			for ip in "${nat_ips[@]}"; do [[ -n "$ip" ]] && PROTECTED_IPS["${ip}"]=1; done
		done
	fi

	# Part 4: Protect TPU VMs based on their name in EXCLUSION_MAP or labels.
	log "INFO" "Checking for TPU VMs to protect..."
	local tpus_data
	if ! tpus_data=$(gcloud compute tpus tpu-vm list --project="$PROJECT_ID" --zone - --format="value(name,ZONE,labels.map())"); then
		log "ERROR" "Failed to list TPU VMs."
		((ERROR_COUNT++)) || true
	else
		if [[ -z "$tpus_data" ]]; then
			log "INFO" "No TPU VMs found in project $PROJECT_ID."
		else
			while IFS=$'\t' read -r tpu_name tpu_zone tpu_labels; do
				[[ -z "$tpu_name" ]] && continue
				log "DEBUG" "Read TPU: name='${tpu_name}', zone='${tpu_zone}', labels='${tpu_labels}'"

				if is_excluded "$tpu_name" "$tpu_labels"; then # Returns 0 if excluded
					if ! [[ -v EXCLUSION_MAP["$tpu_name"] ]]; then
						log "INFO" "Protecting TPU VM '${tpu_name}' in ${tpu_zone} by adding to exclusion map."
						EXCLUSION_MAP["${tpu_name}"]=1
					fi
					if [[ -n "$tpu_zone" ]]; then
						TPUS_TO_PROTECT["${tpu_name}"]="${tpu_zone}"
					else
						log "WARNING" "Zone information missing for protected TPU VM ${tpu_name}."
					fi
				fi
			done <<<"$tpus_data"
		fi
	fi

	# Part 5: Protect network resources associated with the collected TPU VMs.
	if ((${#TPUS_TO_PROTECT[@]} > 0)); then
		log "INFO" "Protecting sub-resources for ${#TPUS_TO_PROTECT[@]} protected TPU VMs..."
		for tpu_name in "${!TPUS_TO_PROTECT[@]}"; do
			local zone="${TPUS_TO_PROTECT[$tpu_name]}"
			if [[ -z "$zone" ]]; then
				log "WARNING" "Skipping network protection for TPU ${tpu_name} due to missing zone."
				continue
			fi

			log "DEBUG" "Fetching details for protected TPU VM: ${tpu_name} in ${zone}"
			local tpu_details
			if ! tpu_details=$(gcloud compute tpus tpu-vm describe "${tpu_name}" \
				--zone="${zone}" --project="${PROJECT_ID}" \
				--format="value(networkConfig.network, networkConfig.subnetwork)"); then
				log "WARNING" "Failed to describe protected TPU VM ${tpu_name} in ${zone}. Its network resources might not be protected."
				continue
			fi

			local net_url sub_url
			IFS=$'\t' read -r net_url sub_url <<<"$tpu_details"
			_protect_network_resources "TPU" "$tpu_name" "$net_url" "$sub_url"
		done
	fi

	# Part 6: Protect Address resources corresponding to PROTECTED_IPS.
	if ((${#PROTECTED_IPS[@]} > 0)); then
		log "INFO" "Protecting Compute Address resources used by protected instances/TPUs..."
		local addresses_data
		if ! addresses_data=$(gcloud compute addresses list --project="$PROJECT_ID" --format="value(name,address)"); then
			log "WARNING" "Failed to list Compute Addresses. Some Address resources might not be protected."
		else
			while IFS=$'\t' read -r addr_name addr_ip; do
				if [[ -n "${addr_ip}" && -n "${PROTECTED_IPS[${addr_ip}]:-}" ]]; then
					if [[ -n "${addr_name}" && -z "${EXCLUSION_MAP[${addr_name}]:-}" ]]; then
						log "INFO" "Protecting Address '${addr_name}' (${addr_ip}) by adding to exclusion map."
						EXCLUSION_MAP["${addr_name}"]=1
					fi
				fi
			done <<<"$addresses_data"
		fi
	fi

	# Part 7: Protect Instance Templates that use protected networks or subnetworks.
	log "INFO" "Checking Instance Templates for usage of protected networks/subnetworks..."
	local templates_data
	if ! templates_data=$(gcloud compute instance-templates list --project="$PROJECT_ID" \
		--format="value(name,properties.networkInterfaces.network.list(separator=';'),properties.networkInterfaces.subnetwork.list(separator=';'))"); then
		log "WARNING" "Failed to list instance templates. Some templates using protected networks might not be protected."
	else
		while IFS=$'\t' read -r template_name nets_str subs_str; do
			[[ -z "$template_name" ]] && continue
			if [[ -v EXCLUSION_MAP["$template_name"] ]]; then
				continue # Already protected
			fi

			local found_protected_net=false
			# Check if any Network URI used by the template is in PROTECTED_NETWORK_URIS
			if [[ -n "$nets_str" ]]; then
				IFS=';' read -ra net_uris <<<"$nets_str"
				for net_uri in "${net_uris[@]}"; do
					if [[ -n "$net_uri" ]] && [[ -v PROTECTED_NETWORK_URIS["$net_uri"] ]]; then
						log "INFO" "Protecting Instance Template '${template_name}' (uses protected network '$(basename "$net_uri")') by adding to exclusion map."
						EXCLUSION_MAP["${template_name}"]=1
						found_protected_net=true
						break
					fi
				done
			fi
			[[ "$found_protected_net" == true ]] && continue

			# Check if any Subnetwork name used by the template is in EXCLUSION_MAP
			if [[ -n "$subs_str" ]]; then
				IFS=';' read -ra sub_uris <<<"$subs_str"
				for sub_uri in "${sub_uris[@]}"; do
					if [[ -n "$sub_uri" ]]; then
						local sub_name
						sub_name=$(basename "$sub_uri")
						if [[ -n "$sub_name" ]] && [[ -v EXCLUSION_MAP["$sub_name"] ]]; then
							log "INFO" "Protecting Instance Template '${template_name}' (uses protected subnetwork '${sub_name}') by adding to exclusion map."
							EXCLUSION_MAP["${template_name}"]=1
							break
						fi
					fi
				done
			fi
		done <<<"$templates_data"
	fi
	log "INFO" "Finished identifying resources to protect."
}

log_exclusion_map() {
	log "INFO" "--- Current Exclusion Map Contents ---"
	if [ ${#EXCLUSION_MAP[@]} -eq 0 ]; then
		log "INFO" "Exclusion map is empty."
		return
	fi
	for key in "${!EXCLUSION_MAP[@]}"; do
		log "INFO" "EXCLUDED: $key"
	done
	log "INFO" "--- End of Exclusion Map ---"
}

log_protected_network_uris() {
	log "INFO" "--- Protected Network URIs ---"
	if [ ${#PROTECTED_NETWORK_URIS[@]} -eq 0 ]; then
		log "INFO" "PROTECTED_NETWORK_URIS map is empty."
		return
	fi
	for key in "${!PROTECTED_NETWORK_URIS[@]}"; do
		log "INFO" "PROTECTED NET URI: $key"
	done
	log "INFO" "--- End of Protected Network URIs ---"
}

# STANDARD RESOURCE PROCESSOR
# Arguments:
#   Label for logging (e.g., "GKE Cluster")
#   Command to list resources (as a string to be eval'd)
#   Scope type for delete command ('zone', 'location', 'region', or 'none')
#   Base delete command components (array)
process_resources() {
	local label="$1"
	local list_command="$2"
	local scope_type="$3" # Should be 'zone' or 'location' or 'region'
	shift 3
	local -a delete_command_base=("$@")

	log "INFO" "--- Processing: $label ---"

	local resources
	if ! resources=$(eval "$list_command"); then
		log "ERROR" "Failed to list $label"
		((ERROR_COUNT++)) || true
		return 0
	fi

	if [[ -z "$resources" ]]; then
		log "INFO" "No $label found matching criteria."
		return 0
	fi

	local count=0
	while IFS=$'\t' read -r name scope labels_str; do
		[[ -z "$name" ]] && continue
		log "DEBUG" "Processing line: name='${name}', scope='${scope}', labels_str='${labels_str}'"

		if ! is_excluded "$name" "${labels_str:-}"; then
			local -a cmd=("${delete_command_base[@]}")
			cmd+=("$name")
			cmd+=("--quiet")
			if [[ "$scope_type" != "none" && -n "$scope" ]]; then
				cmd+=("--$scope_type=$scope")
			fi

			execute_delete "$label" "$name" "($scope)" "${cmd[@]}"
			((count++)) || true
		fi
	done <<<"$resources"
	log "INFO" "Finished processing $label. Actioned $count resources."
}

# SPECIFIC RESOURCE HANDLERS

process_addresses() {
	log "INFO" "--- Processing: Compute Addresses ---"
	local regional_addresses
	# Process Regional Addresses
	if ! regional_addresses=$(gcloud compute addresses list --project="$PROJECT_ID" \
		--filter="creationTimestamp < '$CUTOFF_TIME' AND region:*" \
		--format="value(name,region.basename(),status,labels.map())" | sort); then
		log "ERROR" "Failed to list Regional Addresses."
		((ERROR_COUNT++)) || true
	else
		while IFS=$'\t' read -r name region status labels_str; do
			[[ -z "$name" ]] && continue
			if ! is_excluded "$name" "${labels_str:-}"; then
				if [[ "$status" == "IN_USE" ]]; then
					log "SKIP" "Skipping IN_USE Regional Address $name ($region)."
					continue
				fi
				execute_delete "Regional Address" "$name" "($region)" \
					gcloud compute addresses delete "$name" --project="$PROJECT_ID" --region="$region" --quiet
			fi
		done <<<"$regional_addresses"
	fi

	# Process Global Addresses
	local global_addresses
	if ! global_addresses=$(gcloud compute addresses list --project="$PROJECT_ID" \
		--filter="creationTimestamp < '$CUTOFF_TIME' AND NOT region:*" \
		--format="value(name, status, labels.map())" | sort); then
		log "ERROR" "Failed to list Global Addresses."
		((ERROR_COUNT++)) || true
	else
		while IFS=$'\t' read -r name status labels_str; do
			[[ -z "$name" ]] && continue
			if ! is_excluded "$name" "${labels_str:-}"; then
				if [[ "$status" == "IN_USE" ]]; then
					log "SKIP" "Skipping IN_USE Global Address $name."
					continue
				fi
				execute_delete "Global Address" "$name" "(Global)" \
					gcloud compute addresses delete "$name" --project="$PROJECT_ID" --global --quiet
			fi
		done <<<"$global_addresses"
	fi
}

process_iam_deleted_members() {
	log "INFO" "--- Processing: IAM Role Bindings for Deleted Service Accounts ---"
	local policy_data
	if ! policy_data=$(gcloud projects get-iam-policy "$PROJECT_ID" --format="value(bindings[].role,bindings[].members)"); then
		log "ERROR" "Failed to get IAM policy."
		((ERROR_COUNT++)) || true
		return 0
	fi
	if [[ -z "$policy_data" ]]; then
		log "INFO" "No IAM bindings found."
		return 0
	fi

	local count=0
	while IFS=$'\t' read -r role members_str; do
		if [[ -z "$role" || -z "$members_str" ]]; then continue; fi

		IFS=';' read -ra members <<<"$members_str"
		for member in "${members[@]}"; do
			if [[ "$member" == deleted:serviceAccount:* ]]; then
				local -a cmd=(
					"gcloud" "projects" "remove-iam-policy-binding" "$PROJECT_ID"
					"--member=$member"
					"--role=$role"
					"--condition=None"
					"--quiet"
				)

				if [[ "$DRY_RUN" == "true" ]]; then
					log "DRY-RUN" "Would remove IAM binding: $member from role $role"
				else
					log "EXECUTE" "Removing IAM binding: $member from role $role"
					if ! "${cmd[@]}" >/dev/null; then
						log "ERROR" "Failed to remove IAM binding for $member in role $role"
						((ERROR_COUNT++)) || true
					fi
				fi
				((count++)) || true
			fi
		done
	done <<<"$policy_data"
	log "INFO" "Finished processing IAM deleted members. $count bindings actioned."
}

process_vm_images() {
	log "INFO" "--- Processing: VM Images ---"
	local images
	if ! images=$(gcloud compute images list --project="$PROJECT_ID" --no-standard-images \
		--filter="creationTimestamp < '$CUTOFF_TIME_IMAGES'" \
		--format="value(name,labels.map())"); then
		log "ERROR" "Failed to list VM images"
		((ERROR_COUNT++)) || true
		return 0
	fi
	if [[ -z "$images" ]]; then
		log "INFO" "No custom VM images found matching criteria."
		return 0
	fi

	local count=0
	while IFS=$'\t' read -r name labels_str; do
		[[ -z "$name" ]] && continue
		if ! is_excluded "$name" "${labels_str:-}"; then
			execute_delete "VM Image" "$name" "" \
				gcloud compute images delete "$name" --project="$PROJECT_ID" --quiet
			((count++)) || true
		fi
	done <<<"$images"
	log "INFO" "Finished processing VM Images. $count images actioned."
}

process_docker_images() {
	log "INFO" "--- Processing: Docker Images for 'test-runner' in Artifact Registry ---"
	local cutoff_date
	cutoff_date=$(date -u -d "14 days ago" '+%Y-%m-%dT%H:%M:%SZ')
	local cutoff_seconds
	if ! cutoff_seconds=$(date -u -d "$cutoff_date" +%s); then
		log "ERROR" "Failed to calculate cutoff seconds for Docker image cleanup."
		((ERROR_COUNT++)) || true
		return 0
	fi
	local location="us-central1"
	local repo_name="hpc-toolkit-repo"
	local package_name="test-runner"
	local full_package_url="${location}-docker.pkg.dev/${PROJECT_ID}/${repo_name}/${package_name}"

	local images_output
	if ! images_output=$(gcloud artifacts docker images list "$full_package_url" --format="csv[no-heading](uri,updateTime)" --sort-by="updateTime" 2>/dev/null); then
		log "WARNING" "Failed to list Docker images for $full_package_url. Repository might not exist or be empty."
		return 0
	fi
	if [[ -z "$images_output" ]]; then
		log "INFO" "No Docker image versions found for $package_name in $full_package_url."
		return 0
	fi

	local count=0
	while IFS=, read -r full_image_ref update_time; do
		if [[ -z "$full_image_ref" || -z "$update_time" || "$full_image_ref" != *"@sha256:"* ]]; then continue; fi

		local image_seconds
		if ! image_seconds=$(date -u -d "$update_time" +%s 2>/dev/null); then
			log "WARNING" "Could not parse update time '$update_time' for Docker image $full_image_ref."
			continue
		fi

		if [[ $image_seconds -lt $cutoff_seconds ]]; then
			if ! is_excluded "$package_name" && ! is_excluded "$full_image_ref"; then
				execute_delete "Docker Image Version" "$full_image_ref" "(Updated: $update_time)" \
					gcloud artifacts docker images delete "$full_image_ref" --project="$PROJECT_ID" --delete-tags --quiet
				((count++)) || true
			else
				log "SKIP" "Docker Image Version: $full_image_ref - Skipped because package/repo is protected."
			fi
		fi
	done <<<"$images_output"
	log "INFO" "Finished processing Docker Images. $count versions actioned."
}

process_filestore() {
	log "INFO" "--- Processing: Filestore Instances ---"
	local fs_data
	# Get filestore instance name, location, labels, and network URI
	if ! fs_data=$(gcloud filestore instances list --project="$PROJECT_ID" --filter="createTime < '$CUTOFF_TIME'" \
		--format="value(name.segment(5), name.segment(3), networks[0].network, labels.map())"); then
		log "ERROR" "Failed to list Filestore instances."
		((ERROR_COUNT++)) || true
		return 0
	fi
	if [[ -z "$fs_data" ]]; then
		log "INFO" "No Filestore instances found matching criteria (older than $CUTOFF_TIME)."
		return 0
	fi

	local count=0
	while IFS=$'\t' read -r name location network_uri labels_str; do
		name=$(echo "$name" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
		location=$(echo "$location" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
		network_uri=$(echo "$network_uri" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')

		if [[ -z "$name" || "$name" == "None" || -z "$location" || "$location" == "None" ]]; then
			log "WARNING" "Could not extract valid name or location for a Filestore instance from list output."
			continue
		fi
		log "DEBUG" "Filestore $name: Checking network URI: '$network_uri'"
		# Protect Filestore if its network is used by any protected VM
		local network_basename full_network_uri
		network_basename=$(basename "$network_uri")
		full_network_uri="https://www.googleapis.com/compute/v1/projects/${PROJECT_ID}/global/networks/${network_basename}"

		if [[ -n "${network_uri}" && -n "${PROTECTED_NETWORK_URIS[${full_network_uri}]:-}" ]]; then
			if [[ -z "${EXCLUSION_MAP[${name}]:-}" ]]; then
				log "SKIP" "Filestore $name ($location) on PROTECTED network $network_basename (URI: $full_network_uri)"
				EXCLUSION_MAP["${name}"]=1
			fi
			continue
		else
			if [[ -n "${network_uri}" ]]; then
				log "DEBUG" "Filestore $name: Network URI '$full_network_uri' not found in PROTECTED_NETWORK_URIS."
			else
				log "DEBUG" "Filestore $name: Network URI is empty."
			fi
		fi

		if ! is_excluded "$name" "${labels_str:-}"; then
			log "INFO" "Processing Filestore instance for potential deletion: $name in $location"

			if [[ "$DRY_RUN" == "true" ]]; then
				log "DRY-RUN" "Would disable deletion protection on Filestore: $name ($location)"
				log "DRY-RUN" "Would delete Filestore: $name ($location)"
				((count++)) || true
			else
				log "EXECUTE" "Attempting to disable deletion protection on Filestore: $name ($location)"
				local -a disable_cmd=(
					"gcloud" "filestore" "instances" "update" "$name"
					"--location=$location"
					"--project=$PROJECT_ID"
					"--no-deletion-protection"
					"--quiet"
				)
				if ! "${disable_cmd[@]}"; then
					log "WARNING" "Failed to disable deletion protection for Filestore $name. This might be okay if it was already disabled or the instance is not in a state to be updated. Continuing with delete attempt."
				else
					log "INFO" "Deletion protection disabled for Filestore $name."
				fi

				log "EXECUTE" "Deleting Filestore: $name ($location)"
				local -a delete_cmd=(
					"gcloud" "filestore" "instances" "delete" "$name"
					"--project=$PROJECT_ID"
					"--location=$location"
					"--quiet"
					"--force"
				)
				if "${delete_cmd[@]}"; then
					log "SUCCESS" "Successfully deleted Filestore $name"
					((count++)) || true
				else
					log "ERROR" "Failed to delete Filestore $name"
					((ERROR_COUNT++)) || true
				fi
			fi
		fi
	done <<<"$fs_data"
	log "INFO" "Finished processing Filestore Instances. $count instances actioned."
}

process_subnetworks() {
	log "INFO" "--- Processing: Subnetworks ---"
	local subnets
	if ! subnets=$(gcloud compute networks subnets list --project="$PROJECT_ID" --filter="creationTimestamp < '$CUTOFF_TIME'" --format="value(name,region.basename(),network)"); then
		log "ERROR" "Failed to list Subnetworks."
		((ERROR_COUNT++)) || true
		return 0
	fi

	local count=0
	while IFS=$'\t' read -r name region network_uri; do
		[[ -z "$name" ]] && continue
		local network_name
		network_name=$(basename "$network_uri")
		if [[ "$network_name" == "default" ]]; then
			log "SKIP" "Skipping Subnetwork $name as it belongs to the 'default' network."
			continue
		fi

		if [[ -n "${EXCLUSION_MAP[${network_name}]:-}" ]]; then
			log "SKIP" "Subnetwork: $name - Skipping deletion, as its network '${network_name}' is protected."
			continue
		fi
		if [[ -n "${EXCLUSION_MAP[${name}]:-}" ]]; then
			log "SKIP" "Subnetwork: $name - Skipping deletion, found in exclusion map."
			continue
		fi

		if ! is_excluded "$name"; then
			execute_delete "Subnetwork" "$name" "($region)" \
				gcloud compute networks subnets delete "$name" --project="$PROJECT_ID" --region="$region" --quiet
			((count++)) || true
		fi
	done <<<"$subnets"
	log "INFO" "Finished processing Subnetworks. $count subnetworks actioned."
}

process_networks() {
	log "INFO" "--- Processing: VPC Networks ---"
	local networks
	if ! networks=$(gcloud compute networks list --project="$PROJECT_ID" --filter="creationTimestamp < '$CUTOFF_TIME'" --format="value(name,selfLink)"); then
		log "ERROR" "Failed to list VPC Networks."
		((ERROR_COUNT++)) || true
		return 0
	fi

	local count=0
	while IFS=$'\t' read -r name self_link; do
		[[ -z "$name" || "$name" == "default" || $(is_excluded "$name") ]] && continue
		gcloud compute routes list --project="$PROJECT_ID" --filter="network=\"$self_link\"" --format="value(name)" 2>/dev/null | while IFS= read -r r; do
			if [[ -n "$r" ]] && ! is_excluded "$r"; then
				execute_delete "Dependent Route" "$r" "" \
					gcloud compute routes delete "$r" --project="$PROJECT_ID" --quiet
			fi
		done || true
		# Delete dependent firewall rules
		gcloud compute firewall-rules list --project="$PROJECT_ID" --filter="network=\"$self_link\"" --format="value(name)" 2>/dev/null | while IFS= read -r r; do
			if [[ -n "$r" ]] && ! is_excluded "$r"; then
				execute_delete "Dependent Firewall Rule" "$r" "" \
					gcloud compute firewall-rules delete "$r" --project="$PROJECT_ID" --quiet
			fi
		done || true
		# Delete the network itself
		execute_delete "Network" "$name" "" \
			gcloud compute networks delete "$name" --project="$PROJECT_ID" --quiet
		((count++)) || true
	done <<<"$networks"
	log "INFO" "Finished processing VPC Networks. $count networks actioned."
}

process_routers() {
	log "INFO" "--- Processing: Cloud Routers ---"
	local routers
	if ! routers=$(gcloud compute routers list --project="$PROJECT_ID" \
		--filter="creationTimestamp < '$CUTOFF_TIME'" \
		--format="value(name,region.basename(),network,labels.map())" | sort); then
		log "ERROR" "Failed to list Cloud Routers."
		((ERROR_COUNT++)) || true
		return 0
	fi
	if [[ -z "$routers" ]]; then
		log "INFO" "No Cloud Routers found matching criteria (older than $CUTOFF_TIME)."
		return 0
	fi

	local count=0
	while IFS=$'\t' read -r name region network_uri labels_str; do
		[[ -z "$name" ]] && continue

		local network_name
		network_name=$(basename "$network_uri")
		if [[ -n "${EXCLUSION_MAP[${network_name}]:-}" ]]; then
			log "SKIP" "Cloud Router $name - Network $network_name is protected."
			continue
		fi

		if ! is_excluded "$name" "${labels_str:-}"; then
			execute_delete "Cloud Router" "$name" "($region)" \
				gcloud compute routers delete "$name" --project="$PROJECT_ID" --region="$region" --quiet
			((count++)) || true
		fi
	done <<<"$routers"
	log "INFO" "Finished processing Cloud Routers. $count routers actioned."
}

# MAIN EXECUTION

main() {
	log "INFO" "STARTING RESOURCE CLEANUP for project: $PROJECT_ID"
	log "INFO" "Resource Creation Time Cutoff (General): $CUTOFF_TIME"
	log "INFO" "Resource Creation Time Cutoff (Images): $CUTOFF_TIME_IMAGES"
	log "INFO" "DRY_RUN mode: $DRY_RUN"
	log "INFO" "Exclusion File GCS Path: $EXCLUSION_FILE"

	check_dependencies
	load_exclusions
	populate_protected_resources
	log_protected_network_uris # Log the protected network URIs for debugging
	log_exclusion_map          # Log the final exclusion map for debugging

	# --- Phase 1: High Level Compute Resources (Clusters, TPUs, Instances) ---
	log "INFO" "--- PHASE 1: Deleting high-level compute resources ---"
	process_resources "GKE Cluster" \
		"gcloud container clusters list --project=\"$PROJECT_ID\" --filter=\"createTime < '$CUTOFF_TIME'\" --format=\"value(name,location,resourceLabels.map())\" | sort" \
		"location" \
		"gcloud" "container" "clusters" "delete" "--project=$PROJECT_ID"

	process_resources "TPU VM" \
		"gcloud compute tpus tpu-vm list --project=\"$PROJECT_ID\" --filter=\"createTime < '$CUTOFF_TIME'\" --zone - --format=\"value(name,ZONE,labels.map())\" | sort" \
		"zone" \
		"gcloud" "compute" "tpus" "tpu-vm" "delete" "--project=$PROJECT_ID"

	process_resources "Compute Instance" \
		"gcloud compute instances list --project=\"$PROJECT_ID\" --filter=\"creationTimestamp < '$CUTOFF_TIME'\" --format=\"value(name,zone.basename(),labels.map())\" | sort" \
		"zone" \
		"gcloud" "compute" "instances" "delete" "--project=$PROJECT_ID" "--delete-disks=all"
	process_filestore

	# --- Phase 2: Images & Artifacts (VM Images, Docker Images, Instance Templates) ---
	log "INFO" "--- PHASE 2: Deleting Images and Artifacts ---"
	process_vm_images
	process_docker_images
	process_resources "Instance Template" \
		"gcloud compute instance-templates list --project=\"$PROJECT_ID\" --filter=\"creationTimestamp < '$CUTOFF_TIME'\" --format=\"value(name,'global',labels.map())\" | sort" \
		"none" \
		"gcloud" "compute" "instance-templates" "delete" "--project=$PROJECT_ID"

	# --- Phase 3: Network Infrastructure (Routers, Addresses, Disks) ---
	# Disks are here because they might be detached after instances are deleted.
	log "INFO" "--- PHASE 3: Deleting Network Infrastructure and Disks ---"
	process_routers
	process_addresses
	process_resources "Zonal Disk" \
		"gcloud compute disks list --project=\"$PROJECT_ID\" --filter=\"creationTimestamp < '$CUTOFF_TIME' AND zone:*\" --format=\"value(name,zone.basename(),labels.map())\" | sort" \
		"zone" \
		"gcloud" "compute" "disks" "delete" "--project=$PROJECT_ID"

	# --- Phase 4: Networking Hierarchies (Subnetworks, Networks) ---
	log "INFO" "--- PHASE 4: Deleting Networking Hierarchies ---"
	process_subnetworks
	process_networks # This also handles dependent firewalls and routes

	# --- Phase 5: IAM Cleanup ---
	log "INFO" "--- PHASE 5: Cleaning up IAM Policy Bindings ---"
	process_iam_deleted_members

	log "INFO" "CLEANUP RUN FINISHED for project: $PROJECT_ID"

	if [[ $ERROR_COUNT -gt 0 ]]; then
		log "WARNING" "Finished with $ERROR_COUNT errors during execution."
		exit 1
	else
		log "SUCCESS" "Finished successfully with 0 errors."
		exit 0
	fi
}

main
