#!/bin/bash

# CONFIGURATION & GLOBAL VARIABLES

# Associative array for exclusions
declare -A EXCLUSION_MAP
ERROR_COUNT=0

# To store IPs of protected instances, to find matching Address resources
declare -A PROTECTED_IPS
# To store Network URIs used by protected instances
declare -A PROTECTED_NETWORK_URIS

# HELPER FUNCTIONS

log() {
	local level="$1"
	local message="$2"
	echo "[$(date +'%Y-%m-%d %H:%M:%S')] [$level] $message"
}

check_dependencies() {
	local dependencies=("gcloud" "awk" "grep" "sort" "date" "sed" "basename")
	for cmd in "${dependencies[@]}"; do
		if ! command -v "$cmd" &>/dev/null; then
			log "ERROR" "Missing required dependency: $cmd"
			exit 1 # Dependencies are critical, we must exit immediately here.
		fi
	done
}

load_exclusions() {
	log "INFO" "Loading exclusions from $EXCLUSION_FILE..."

	local line_count=0
	# Helper function to process each line from the exclusion source
	process_line() {
		local line="$1"
		local trimmed_line
		trimmed_line=$(echo "$line" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
		if [[ -n "$trimmed_line" ]] && [[ "$trimmed_line" != \#* ]]; then
			if [[ -z "${EXCLUSION_MAP[${trimmed_line}]:-}" ]]; then
				EXCLUSION_MAP["${trimmed_line}"]=1
				((line_count++))
			fi
		fi
	}

	log "INFO" "Exclusion file is a GCS path. Streaming content..."
	# Preliminary check to see if the GCS object(exclusion file) exists and is accessible
	if ! gcloud storage ls "$EXCLUSION_FILE" >/dev/null 2>&1; then
		log "ERROR" "Cannot access GCS exclusion file: $EXCLUSION_FILE. Please check the path and permissions."
		exit 1
	fi
	while IFS= read -r line || [[ -n "$line" ]]; do
		process_line "$line"
	done < <(gcloud storage cat "$EXCLUSION_FILE")

	if [[ ${#EXCLUSION_MAP[@]} -eq 0 ]]; then
		log "ERROR" "No valid exclusion entries loaded from $EXCLUSION_FILE. Exiting to prevent accidental deletion."
		exit 1
	else
		log "INFO" "Loaded ${#EXCLUSION_MAP[@]} unique exclusion entries."
	fi
}

# Returns 0 if EXCLUDED (DO NOT delete)
# Returns 1 if NOT excluded (OK to delete)
is_excluded() {
	local resource_name="$1"
	local labels_str="${2:-}" # Expected format: key1=value1;key2=value2

	if [[ -n "${EXCLUSION_MAP[${resource_name}]:-}" ]]; then
		log "SKIP" "$resource_name (In Exclusion Map)"
		return 0 # Excluded
	fi

	if [[ -n "$labels_str" ]]; then
		IFS=';' read -ra LABEL_PAIRS <<<"$labels_str"
		for PAIR in "${LABEL_PAIRS[@]}"; do
			local KEY VAL
			KEY="${PAIR%%=*}"
			VAL="${PAIR#*=}"
			if [[ "$KEY" == "cleanup-exemption-date" ]]; then
				if [[ "$VAL" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
					local exp_seconds
					if ! exp_seconds=$(date -d "$VAL + 1 day" -u +%s 2>/dev/null); then
						log "WARNING" "$resource_name (Label: cleanup-exemption-date invalid date value: $VAL)"
						return 1 # Not excluded
					else
						local current_seconds
						current_seconds=$(date -u +%s)
						if [[ "$exp_seconds" -gt "$current_seconds" ]]; then
							log "SKIP" "$resource_name (Label: cleanup-exemption-date=$VAL, valid)"
							return 0 # Excluded
						else
							log "INFO" "$resource_name (Label: cleanup-exemption-date=$VAL expired)"
							return 1 # Not excluded
						fi
					fi
				else
					log "WARNING" "$resource_name (Label: cleanup-exemption-date invalid date format: $VAL, expected YYYY-MM-DD)"
					return 1 # Not excluded
				fi
				break
			fi
		done
	fi
	return 1 # Not excluded
}

execute_delete() {
	local resource_type="$1"
	local resource_name="$2"
	local cmd_str="$3"
	local extra_info="${4:-}"

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN" "Would delete $resource_type: $resource_name $extra_info"
	else
		log "EXECUTE" "Deleting $resource_type: $resource_name $extra_info"
		if eval "$cmd_str"; then
			log "SUCCESS" "Deleted $resource_name"
		else
			log "ERROR" "Failed to delete $resource_name"
			((ERROR_COUNT++)) || true
		fi
	fi
}

# Helper function to add network/subnetwork to exclusion lists
_protect_network_resources() {
	local source_resource_type="$1"
	local source_resource_name="$2"
	local net_url="$3"
	local sub_url="$4"

	# Protect Network
	if [[ -n "$net_url" && "$net_url" != "None" ]]; then
		local net_name=$(basename "${net_url}")
		if [[ -n "${net_name}" && -z "${EXCLUSION_MAP[${net_name}]:-}" ]]; then
			log "INFO" "Excluding Network (for ${source_resource_type} ${source_resource_name}): ${net_name}"
			EXCLUSION_MAP["${net_name}"]=1
		fi
		# Store the full network URI for Filestore matching
		log "DEBUG" "Adding protected network URI (for ${source_resource_type} ${source_resource_name}): ${net_url}"
		PROTECTED_NETWORK_URIS["${net_url}"]=1
	fi

	# Protect Subnetwork
	if [[ -n "$sub_url" && "$sub_url" != "None" ]]; then
		local sub_name=$(basename "${sub_url}")
		if [[ -n "${sub_name}" && -z "${EXCLUSION_MAP[${sub_name}]:-}" ]]; then
			log "INFO" "Excluding Subnetwork (for ${source_resource_type} ${source_resource_name}): ${sub_name}"
			EXCLUSION_MAP["${sub_name}"]=1
		fi
	fi
}

populate_protected_resources() {
	log "INFO" "Identifying protected resources..."
	declare -A INSTANCES_TO_PROTECT # Map instance_name -> zone
	declare -A TPUS_TO_PROTECT      # Map tpu_name -> zone

	# Part 1: Instances and Templates from EXCLUDED GKE clusters
	log "INFO" "Checking for instances in EXCLUDED GKE clusters..."
	local clusters_data
	if ! clusters_data=$(gcloud container clusters list --project="$PROJECT_ID" --format="value(name,location,resourceLabels.map())"); then
		log "ERROR" "Failed to list GKE clusters."
		((ERROR_COUNT++)) || true
	else
		while IFS=$'\t' read -r cluster_name location labels_str; do
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
					local ig_name=$(basename "${ig_url}")
					local ig_scope_type=$(echo "${ig_url}" | awk -F'/' '{print $(NF-3)}')
					local ig_scope_name=$(echo "${ig_url}" | awk -F'/' '{print $(NF-2)}')
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
							local template_name=$(basename "$template_url_from_mig")
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
							local inst_zone=$(basename "$inst_zone_url")
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

	# Part 2: Instances protected via direct labels or name
	log "INFO" "Checking for instances to protect..."
	local instances_data
	if ! instances_data=$(gcloud compute instances list \
		--project="$PROJECT_ID" \
		--format="value(name,zone.basename(),labels.map())"); then
		log "ERROR" "Failed to list instances."
		((ERROR_COUNT++)) || true
	else
		while IFS=$'\t' read -r inst_name zone labels_str; do
			if is_excluded "$inst_name" "$labels_str"; then # Returns 0 if excluded
				if ! [[ -v EXCLUSION_MAP["$inst_name"] ]]; then
					log "INFO" "Protecting Instance: ${inst_name} in ${zone}"
					EXCLUSION_MAP["${inst_name}"]=1
				fi
				INSTANCES_TO_PROTECT["${inst_name}"]="${zone}"
			fi
		done <<<"$instances_data"
	fi

	# Part 3: Protect resources associated with the collected instances
	if ((${#INSTANCES_TO_PROTECT[@]} > 0)); then
		log "INFO" "Protecting sub-resources of ${#INSTANCES_TO_PROTECT[@]} instances..."
		for inst_name in "${!INSTANCES_TO_PROTECT[@]}"; do
			local zone="${INSTANCES_TO_PROTECT[$inst_name]}"
			log "DEBUG" "Fetching details for protected instance: ${inst_name} in ${zone}"
			local inst_details
			if ! inst_details=$(gcloud compute instances describe "${inst_name}" --zone="${zone}" --project="${PROJECT_ID}" \
				--format="value(disks[].source.list(separator=';'),networkInterfaces[].network.list(separator=';'),networkInterfaces[].subnetwork.list(separator=';'),networkInterfaces[].networkIP.list(separator=';'),networkInterfaces[].accessConfigs[].natIP.list(separator=';'),sourceInstanceTemplate)"); then
				log "WARNING" "Failed to describe protected instance ${inst_name} in ${zone}. Sub-resources might not be protected."
				continue
			fi

			local disks_list nets_list subs_list ips_list nat_ips_list template_url
			IFS=$'\t' read -r disks_list nets_list subs_list ips_list nat_ips_list template_url <<<"$inst_details"

			log "DEBUG" "Instance ${inst_name}: sourceInstanceTemplate value: '${template_url}'"

			# Protect Instance Template (for non-MIG instances)
			if [[ -n "$template_url" ]]; then
				local template_name=$(basename "$template_url")
				if [[ -n "$template_name" && "$template_name" != "None" ]]; then
					if ! [[ -v EXCLUSION_MAP["$template_name"] ]]; then
						log "INFO" "Excluding Instance Template (for ${inst_name}): ${template_name}"
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
				local disk_name=$(basename "${disk_url}")
				if [[ -n "${disk_name}" && -z "${EXCLUSION_MAP[${disk_name}]:-}" ]]; then
					log "INFO" "Excluding Disk (for ${inst_name}): ${disk_name}"
					EXCLUSION_MAP["${disk_name}"]=1
				fi
			done

			# Network & Subnetwork Protection
			IFS=';' read -ra net_urls <<<"$nets_list"
			IFS=';' read -ra sub_urls <<<"$subs_list"
			for i in "${!net_urls[@]}"; do
				_protect_network_resources "Instance" "$inst_name" "${net_urls[$i]:-}" "${sub_urls[$i]:-}"
			done

			# Collect IPs to protect Addresses later
			IFS=';' read -ra network_ips <<<"$ips_list"
			for ip in "${network_ips[@]}"; do [[ -n "$ip" ]] && PROTECTED_IPS["${ip}"]=1; done
			IFS=';' read -ra nat_ips <<<"$nat_ips_list"
			for ip in "${nat_ips[@]}"; do [[ -n "$ip" ]] && PROTECTED_IPS["${ip}"]=1; done
		done
	fi

	# Part 4: TPU VMs protected via direct labels or name
	log "INFO" "Checking for TPU VMs to protect..."
	local tpus_data
	if ! tpus_data=$(gcloud compute tpus tpu-vm list --project="$PROJECT_ID" --zone - --format="value(name,ZONE,labels.map())"); then
		log "ERROR" "Failed to list TPU VMs."
		((ERROR_COUNT++)) || true
	else
		while IFS=$'\t' read -r tpu_name tpu_zone tpu_labels; do
			[[ -z "$tpu_name" ]] && continue

			log "DEBUG" "Parsed TPU: name='${tpu_name}', zone='${tpu_zone}'"

			if is_excluded "$tpu_name" "$tpu_labels"; then
				if ! [[ -v EXCLUSION_MAP["$tpu_name"] ]]; then
					log "INFO" "Protecting TPU VM: ${tpu_name} in ${tpu_zone}"
					EXCLUSION_MAP["${tpu_name}"]=1
				fi
				if [[ -n "$tpu_zone" ]]; then
					TPUS_TO_PROTECT["${tpu_name}"]="${tpu_zone}"
				fi
			fi
		done <<<"$tpus_data"
	fi

	# Part 5: Protect resources associated with the collected TPU VMs
	if ((${#TPUS_TO_PROTECT[@]} > 0)); then
		log "INFO" "Protecting sub-resources of ${#TPUS_TO_PROTECT[@]} TPU VMs..."
		for tpu_name in "${!TPUS_TO_PROTECT[@]}"; do
			local zone="${TPUS_TO_PROTECT[$tpu_name]}"
			if [[ -z "$zone" ]]; then
				log "WARNING" "Skipping sub-resource protection for TPU ${tpu_name} due to missing zone."
				continue
			fi

			log "DEBUG" "Fetching details for protected TPU VM: ${tpu_name} in ${zone}"
			local tpu_details
			if ! tpu_details=$(gcloud compute tpus tpu-vm describe "${tpu_name}" \
				--zone="${zone}" --project="${PROJECT_ID}" \
				--format="value(networkConfig.network, networkConfig.subnetwork)"); then
				log "WARNING" "Failed to describe protected TPU VM ${tpu_name} in ${zone}. Sub-resources might not be protected."
				continue
			fi

			local net_url sub_url
			IFS=$'\t' read -r net_url sub_url <<<"$tpu_details"
			_protect_network_resources "TPU" "$tpu_name" "$net_url" "$sub_url"
		done
	fi

	# Part 6: Find Address resource names for the PROTECTED_IPS
	if ((${#PROTECTED_IPS[@]} > 0)); then
		log "INFO" "Finding Address resource names for protected IPs..."
		local addresses_data
		if ! addresses_data=$(gcloud compute addresses list --project="$PROJECT_ID" --format="value(name,address)"); then
			log "WARNING" "Failed to list addresses to protect by IP."
		else
			while IFS=$'\t' read -r addr_name addr_ip; do
				if [[ -n "${addr_ip}" && -n "${PROTECTED_IPS[${addr_ip}]:-}" ]]; then
					if [[ -n "${addr_name}" && -z "${EXCLUSION_MAP[${addr_name}]:-}" ]]; then
						log "INFO" "Excluding Address: ${addr_name} (${addr_ip})"
						EXCLUSION_MAP["${addr_name}"]=1
					fi
				fi
			done <<<"$addresses_data"
		fi
	fi

	# Part 7: Protect Instance Templates based on Network Configuration (No jq)
	log "INFO" "Checking Instance Templates for usage of protected networks..."
	local templates_data
	if ! templates_data=$(gcloud compute instance-templates list --project="$PROJECT_ID" \
		--format="value(name,properties.networkInterfaces.network.list(separator=';'),properties.networkInterfaces.subnetwork.list(separator=';'))"); then
		log "WARNING" "Failed to list instance templates for network-based protection."
	else
		while IFS=$'\t' read -r template_name nets_str subs_str; do
			if [[ -z "$template_name" ]]; then
				continue
			fi

			# Skip if template is already excluded
			if [[ -v EXCLUSION_MAP["$template_name"] ]]; then
				continue
			fi

			local found_protected=false

			# Check Network URIs
			if [[ -n "$nets_str" ]]; then
				IFS=';' read -ra net_uris <<<"$nets_str"
				for net_uri in "${net_uris[@]}"; do
					if [[ -n "$net_uri" ]] && [[ -v PROTECTED_NETWORK_URIS["$net_uri"] ]]; then
						local net_name=$(basename "$net_uri")
						log "INFO" "Excluding Instance Template (uses protected network '${net_name}'): ${template_name}"
						EXCLUSION_MAP["${template_name}"]=1
						found_protected=true
						break
					fi
				done
			fi

			if [[ "$found_protected" = true ]]; then
				continue # Move to the next template
			fi

			# Check Subnetwork URIs
			if [[ -n "$subs_str" ]]; then
				IFS=';' read -ra sub_uris <<<"$subs_str"
				for sub_uri in "${sub_uris[@]}"; do
					if [[ -n "$sub_uri" ]]; then
						local sub_name=$(basename "$sub_uri")
						if [[ -n "$sub_name" ]] && [[ -v EXCLUSION_MAP["$sub_name"] ]]; then
							log "INFO" "Excluding Instance Template (uses protected subnetwork '${sub_name}'): ${template_name}"
							EXCLUSION_MAP["${template_name}"]=1
							break
						fi
					fi
				done
			fi

		done <<<"$templates_data"
	fi
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

# STANDARD PROCESSOR

process_resources() {
	local label="$1"
	local list_command="$2"
	local delete_command_base="$3"
	local scope_type="$4" # Should be 'zone' or 'location' or 'region'

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
	while IFS=',' read -r name scope labels_str; do
		# Trim potential whitespace and quotes from CSV
		name=$(echo "$name" | sed -e 's/^[[:space:]"]*//' -e 's/[[:space:]"]*$//')
		scope=$(echo "$scope" | sed -e 's/^[[:space:]"]*//' -e 's/[[:space:]"]*$//')
		labels_str=$(echo "$labels_str" | sed -e 's/^[[:space:]"]*//' -e 's/[[:space:]"]*$//')

		[[ -z "$name" ]] && continue

		if ! is_excluded "$name" "${labels_str:-}"; then
			local final_cmd="$delete_command_base \"$name\" --quiet"
			if [[ "$scope_type" != "none" && -n "$scope" ]]; then
				final_cmd="$final_cmd --$scope_type=\"$scope\""
			fi

			execute_delete "$label" "$name" "$final_cmd" "($scope)"
			((count++)) || true
		fi
	done <<<"$resources"
	log "INFO" "Finished processing $label. $count resources actioned."
}

# SPECIFIC HANDLERS

process_addresses() {
	log "INFO" "--- Processing: Compute Addresses ---"
	# Regional Addresses
	local regional_addresses
	if ! regional_addresses=$(gcloud compute addresses list --project="$PROJECT_ID" \
		--filter="creationTimestamp < '$CUTOFF_TIME' AND region:*" \
		--format="value(name,region.basename(),labels.map(),status)" | sort); then
		log "ERROR" "Failed to list Regional Addresses."
		((ERROR_COUNT++)) || true
	else
		while IFS=$'\t' read -r name region labels_str status; do
			[[ -z "$name" ]] && continue
			if ! is_excluded "$name" "${labels_str:-}"; then
				if [[ "$status" == "IN_USE" ]]; then
					log "WARNING" "Skipping IN_USE Regional Address $name ($region) NOT explicitly excluded."
					continue
				fi
				execute_delete "Regional Address" "$name" \
					"gcloud compute addresses delete \"$name\" --project=\"$PROJECT_ID\" --region=\"$region\" --quiet" \
					"($region)"
			fi
		done <<<"$regional_addresses"
	fi

	# Global Addresses
	local global_addresses
	if ! global_addresses=$(gcloud compute addresses list --project="$PROJECT_ID" \
		--filter="creationTimestamp < '$CUTOFF_TIME' AND NOT region:*" \
		--format="value(name, labels.map(), status)" | sort); then
		log "ERROR" "Failed to list Global Addresses."
		((ERROR_COUNT++)) || true
	else
		while IFS=$'\t' read -r name labels_str status; do
			[[ -z "$name" ]] && continue
			if ! is_excluded "$name" "${labels_str:-}"; then
				if [[ "$status" == "IN_USE" ]]; then
					log "DEBUG" "Skipping IN_USE Global Address $name NOT explicitly excluded."
					continue
				fi
				execute_delete "Global Address" "$name" \
					"gcloud compute addresses delete \"$name\" --project=\"$PROJECT_ID\" --global --quiet" \
					"(Global)"
			fi
		done <<<"$global_addresses"
	fi
}

process_iam_deleted_members() {
	log "INFO" "--- Processing: IAM Role Bindings for Deleted SAs ---"
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
				local cmd="gcloud projects remove-iam-policy-binding \"$PROJECT_ID\" --member=\"$member\" --role=\"$role\" --condition=None --quiet"

				if [[ "$DRY_RUN" == "true" ]]; then
					log "DRY-RUN" "Would remove IAM binding: $member from role $role"
				else
					log "EXECUTE" "Removing IAM binding: $member from role $role"
					if ! eval "$cmd" >/dev/null; then
						log "ERROR" "Failed to remove binding for $member in $role"
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
		--format="value(name,creationTimestamp,labels.map())"); then
		log "ERROR" "Failed to list VM images"
		((ERROR_COUNT++)) || true
		return 0
	fi
	if [[ -z "$images" ]]; then
		log "INFO" "No custom VM images found matching criteria."
		return 0
	fi

	local count=0
	while IFS=$'\t' read -r name timestamp labels_str; do
		[[ -z "$name" ]] && continue
		if ! is_excluded "$name" "${labels_str:-}"; then
			execute_delete "VM Image" "$name" \
				"gcloud compute images delete \"$name\" --project=\"$PROJECT_ID\" --quiet"
			((count++)) || true
		fi
	done <<<"$images"
}

process_docker_images() {
	log "INFO" "--- Processing: Docker Images for 'test-runner' (Artifact Registry) ---"
	local cutoff_date=$(date -u -d "14 days ago" '+%Y-%m-%dT%H:%M:%SZ')
	local cutoff_seconds
	if ! cutoff_seconds=$(date -u -d "$cutoff_date" +%s); then
		log "ERROR" "Failed to calculate cutoff_seconds."
		((ERROR_COUNT++)) || true
		return 0
	fi
	local location="us-central1"
	local repo_name="hpc-toolkit-repo"
	local package_name="test-runner"
	local full_package_url="${location}-docker.pkg.dev/${PROJECT_ID}/${repo_name}/${package_name}"
	local images_output
	if ! images_output=$(gcloud artifacts docker images list "$full_package_url" --format="csv[no-heading](uri,updateTime)" --sort-by="updateTime" 2>/dev/null); then
		log "WARNING" "Failed to list images for $full_package_url (Repo might not exist or empty)"
		return 0
	fi
	if [[ -z "$images_output" ]]; then
		log "INFO" "No image versions found for $package_name."
		return 0
	fi
	local count=0
	while IFS=, read -r full_image_ref update_time; do
		if [[ -z "$full_image_ref" || -z "$update_time" || "$full_image_ref" != *"@sha256:"* ]]; then continue; fi
		local image_seconds
		if ! image_seconds=$(date -u -d "$update_time" +%s 2>/dev/null); then continue; fi
		if [[ $image_seconds -lt $cutoff_seconds ]]; then
			if ! is_excluded "$package_name" && ! is_excluded "$full_image_ref"; then
				execute_delete "Docker Image Version" "$full_image_ref" \
					"gcloud artifacts docker images delete \"$full_image_ref\" --project=\"$PROJECT_ID\" --delete-tags --quiet" \
					"(Updated: $update_time)"
				((count++)) || true
			fi
		fi
	done <<<"$images_output"
}

process_filestore() {
	log "INFO" "--- Processing: Filestore Instances ---"
	local fs_data
	# Get instance name, location, labels, and network URI
	if ! fs_data=$(gcloud filestore instances list --project="$PROJECT_ID" --filter="createTime < '$CUTOFF_TIME'" \
		--format="value(name.segment(5), name.segment(3), labels.map(), networks[0].network)"); then
		log "ERROR" "Failed to list Filestore instances."
		((ERROR_COUNT++)) || true
		return 0
	fi
	if [[ -z "$fs_data" ]]; then
		log "INFO" "No Filestore instances found matching criteria."
		return 0
	fi

	local count=0
	while IFS=$'\t' read -r name location labels_str network_uri; do
		name=$(echo "$name" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
		location=$(echo "$location" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
		network_uri=$(echo "$network_uri" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')

		if [[ -z "$name" || "$name" == "None" || -z "$location" || "$location" == "None" ]]; then
			log "WARNING" "Could not extract valid name or location for a Filestore instance."
			continue
		fi

		log "DEBUG" "Filestore $name: Checking network URI: '$network_uri'"
		# Protect Filestore if its network is used by any protected VM
		local network_basename=$(basename "$network_uri")
		# Construct the expected full URI format similar to compute instances
		local full_network_uri="https://www.googleapis.com/compute/v1/projects/${PROJECT_ID}/global/networks/${network_basename}"

		if [[ -n "${network_uri}" && -n "${PROTECTED_NETWORK_URIS[${full_network_uri}]:-}" ]]; then
			if [[ -z "${EXCLUSION_MAP[${name}]:-}" ]]; then
				log "INFO" "SKIP : Filestore $name ($location) on PROTECTED network $network_basename (URI: $full_network_uri)"
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
				local disable_cmd="gcloud filestore instances update \"$name\" --location=\"$location\" --project=\"$PROJECT_ID\" --no-deletion-protection --quiet"
				if ! eval "$disable_cmd"; then
					log "WARNING" "Failed to disable deletion protection for $name. This may be OK if it was already disabled or the instance is not in a state to be updated. Continuing with delete attempt."
				else
					log "INFO" "Deletion protection update command executed successfully for $name"
				fi

				log "EXECUTE" "Deleting Filestore: $name ($location)"
				local delete_cmd="gcloud filestore instances delete \"$name\" --project=\"$PROJECT_ID\" --location=\"$location\" --quiet --force"
				if eval "$delete_cmd"; then
					log "SUCCESS" "Deleted Filestore $name"
					((count++)) || true
				else
					log "ERROR" "Failed to delete Filestore $name"
					((ERROR_COUNT++)) || true
				fi
			fi
		fi
	done <<<"$fs_data"
}

process_subnetworks() {
	log "INFO" "--- Processing: Subnetworks ---"
	local subnets
	if ! subnets=$(gcloud compute networks subnets list --project="$PROJECT_ID" --filter="creationTimestamp < '$CUTOFF_TIME'" --format="value(name,region.basename(),network)"); then
		log "ERROR" "Failed to list subnets"
		((ERROR_COUNT++)) || true
		return 0
	fi

	local count=0
	while IFS=$'\t' read -r name region network_uri; do
		[[ -z "$name" ]] && continue
		local network_name
		network_name=$(basename "$network_uri")
		if [[ "$network_name" == "default" ]]; then continue; fi

		if [[ -n "${EXCLUSION_MAP[${network_name}]:-}" ]]; then
			log "SKIP" "Subnetwork $name - Network $network_name is protected."
			continue
		fi
		if [[ -n "${EXCLUSION_MAP[${name}]:-}" ]]; then
			log "SKIP" "Subnetwork $name - Explicitly protected."
			continue
		fi

		if ! is_excluded "$name"; then
			execute_delete "Subnetwork" "$name" "gcloud compute networks subnets delete \"$name\" --project=\"$PROJECT_ID\" --region=\"$region\" --quiet"
			((count++)) || true
		fi
	done <<<"$subnets"
}

process_networks() {
	log "INFO" "--- Processing: VPC Networks ---"
	local networks
	if ! networks=$(gcloud compute networks list --project="$PROJECT_ID" --filter="creationTimestamp < '$CUTOFF_TIME'" --format="value(name,selfLink)"); then
		log "ERROR" "Failed to list networks"
		((ERROR_COUNT++)) || true
		return 0
	fi

	local count=0
	while IFS=$'\t' read -r name self_link; do
		[[ -z "$name" ]] && continue
		if [[ "$name" == "default" ]]; then continue; fi
		if ! is_excluded "$name"; then
			local routes
			routes=$(gcloud compute routes list --project="$PROJECT_ID" --filter="network=\"$self_link\"" --format="value(name)" 2>/dev/null || true)
			for r in $routes; do if ! is_excluded "$r"; then execute_delete "Dep. Route" "$r" "gcloud compute routes delete \"$r\" --project=\"$PROJECT_ID\" --quiet"; fi; done

			local fws
			fws=$(gcloud compute firewall-rules list --project="$PROJECT_ID" --filter="network=\"$self_link\"" --format="value(name)" 2>/dev/null || true)
			for fw in $fws; do if ! is_excluded "$fw"; then execute_delete "Dep. FW" "$fw" "gcloud compute firewall-rules delete \"$fw\" --project=\"$PROJECT_ID\" --quiet"; fi; done

			execute_delete "Network" "$name" "gcloud compute networks delete \"$name\" --project=\"$PROJECT_ID\" --quiet"
			((count++)) || true
		fi
	done <<<"$networks"
}

process_routers() {
	log "INFO" "--- Processing: Cloud Routers ---"
	local routers
	if ! routers=$(gcloud compute routers list --project="$PROJECT_ID" \
		--filter="creationTimestamp < '$CUTOFF_TIME'" \
		--format="value(name,region.basename(),network,labels.map())" | sort); then
		log "ERROR" "Failed to list Cloud Routers"
		((ERROR_COUNT++)) || true
		return 0
	fi
	if [[ -z "$routers" ]]; then
		log "INFO" "No Cloud Routers found matching criteria."
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
			execute_delete "Cloud Router" "$name" \
				"gcloud compute routers delete \"$name\" --project=\"$PROJECT_ID\" --region=\"$region\" --quiet" \
				"($region)"
			((count++)) || true
		fi
	done <<<"$routers"
}

# MAIN EXECUTION

main() {
	log "INFO" "STARTING RESOURCE CLEANUP: $PROJECT_ID"
	log "INFO" "Time Cutoff (General): $CUTOFF_TIME"
	log "INFO" "Time Cutoff (Images): $CUTOFF_TIME_IMAGES"
	log "INFO" "DRY_RUN: $DRY_RUN"
	log "INFO" "Exclusion File: $EXCLUSION_FILE"

	check_dependencies
	load_exclusions
	populate_protected_resources
	log_protected_network_uris # Log the protected network URIs
	log_exclusion_map          # Log the map contents

	# --- Phase 1: High Level Compute Resources ---
	process_resources "GKE Cluster" \
		"gcloud container clusters list --project=\"$PROJECT_ID\" --filter=\"createTime < '$CUTOFF_TIME'\" --format=\"csv[no-heading](name,location,resourceLabels.map())\" | sort" \
		"gcloud container clusters delete --project=\"$PROJECT_ID\"" "location"

	process_resources "TPU VM" \
		"gcloud compute tpus tpu-vm list --project=\"$PROJECT_ID\" --filter=\"createTime < '$CUTOFF_TIME'\" --zone - --format=\"csv[no-heading](name,ZONE,labels.map())\" | sort" \
		"gcloud compute tpus tpu-vm delete --project=\"$PROJECT_ID\"" "zone"
	process_resources "Compute Instance" \
		"gcloud compute instances list --project=\"$PROJECT_ID\" --filter=\"creationTimestamp < '$CUTOFF_TIME'\" --format=\"csv[no-heading](name,zone.basename(),labels.map())\" | sort" \
		"gcloud compute instances delete --project=\"$PROJECT_ID\" --delete-disks=all" "zone"
	process_filestore

	# --- Phase 2: Images & Artifacts ---
	process_vm_images
	process_docker_images
	process_resources "Instance Template" \
		"gcloud compute instance-templates list --project=\"$PROJECT_ID\" --filter=\"creationTimestamp < '$CUTOFF_TIME'\" --format=\"csv[no-heading](name,'global',labels.map())\" | sort" \
		"gcloud compute instance-templates delete --project=\"$PROJECT_ID\"" "none"

	# --- Phase 3: Network Infrastructure ---
	process_routers
	process_addresses
	process_resources "Zonal Disk" \
		"gcloud compute disks list --project=\"$PROJECT_ID\" --filter=\"creationTimestamp < '$CUTOFF_TIME' AND zone:*\" --format=\"csv[no-heading](name,zone.basename(),labels.map())\" | sort" \
		"gcloud compute disks delete --project=\"$PROJECT_ID\"" "zone"

	# --- Phase 4: Networking Hierarchies ---
	process_subnetworks
	process_networks # This also handles dependent firewalls and routes

	# --- Phase 5: IAM Cleanup ---
	process_iam_deleted_members

	log "INFO" "CLEANUP RUN FINISHED"

	if [[ $ERROR_COUNT -gt 0 ]]; then
		log "WARNING" "Finished with $ERROR_COUNT errors during execution."
		exit 1
	else
		log "SUCCESS" "Finished with 0 errors."
		exit 0
	fi
}

main
