#!/bin/bash
# Copyright 2026 Google LLC
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

# shellcheck disable=SC2317
# known issue that shellcheck wrongly detects trap function as unreachable

set -e -o pipefail

# Trap handler to ensure the Filestore API is always re-enabled on script exit
# after being temporarily disabled to reset internal project limits.
# shellcheck disable=SC2329
function enable_filestore_api() {
	status=$?
	echo "Re-enabling Filestore API..."
	gcloud services enable file.googleapis.com --project "${PROJECT_ID}"
	exit "$status"
}

BUILD_ID=${BUILD_ID:-non-existent-build}
PROJECT_ID=${PROJECT_ID:-$(gcloud config get-value project)}
# Supports DRY_RUN passed from YAML (defaults to false if run directly)
DRY_RUN=${DRY_RUN:-false}

if [ -z "$PROJECT_ID" ]; then
	echo "ERROR: PROJECT_ID must be defined"
	exit 1
fi

<<<<<<< Updated upstream
=======
<<<<<<< Updated upstream
ACTIVE_FILESTORE=$(gcloud filestore instances list --project "${PROJECT_ID}" | tail -n +2 2>/dev/null)
if [[ -n "$ACTIVE_FILESTORE" ]]; then
	echo "Deleting filestore instances"
	while read -r row; do
		# get first two columns: INSTANCE_NAME and LOCATION
		read -ra cols <<<"$row"
		echo "Disabling deletion protection for ${cols[0]} at ${cols[1]}"
		gcloud --project "${PROJECT_ID}" filestore instances update "${cols[0]}" --location="${cols[1]}" --no-deletion-protection --quiet || true
		echo "Deleting ${cols[0]} at ${cols[1]}"
		gcloud --project "${PROJECT_ID}" filestore instances delete --force --quiet --location="${cols[1]}" "${cols[0]}"
	done <<<"$ACTIVE_FILESTORE"
=======
>>>>>>> Stashed changes
# Resources older than 24 hours are considered orphaned and deleted unconditionally.
CLEANUP_AGE_SECONDS=$((24 * 60 * 60))
CURRENT_TIME=$(date +%s)
KEPT_ACTIVE_INSTANCE=false
<<<<<<< Updated upstream
=======
ACTIVE_BUILDS=$(gcloud builds list \
	--project "${PROJECT_ID}" \
	--filter="tags=m.filestore" \
	--format="value(id)" \
	--ongoing 2>/dev/null || true)
>>>>>>> Stashed changes

echo "Starting Filestore & Peering Cleanup for project: ${PROJECT_ID} (DRY_RUN=${DRY_RUN})"
if [ "$DRY_RUN" = "true" ]; then
	echo "SIMULATION ONLY: No resources will actually be deleted."
<<<<<<< Updated upstream
=======
>>>>>>> Stashed changes
>>>>>>> Stashed changes
fi

# Step 1: Clean up Filestore instances.
# Instances >= 24 hours old are deleted immediately as abandoned orphans.
# Instances < 24 hours old are only deleted if no active Filestore Cloud Build is running.
FILESTORE_INSTANCES=$(gcloud filestore instances list \
	--project "${PROJECT_ID}" \
	--format="value(name.basename(),name.segment(3),createTime)" 2>/dev/null || true)

if [[ -n "$FILESTORE_INSTANCES" ]]; then
	while read -r instance location create_time; do
		[[ -z "$instance" ]] && continue

<<<<<<< Updated upstream
=======
<<<<<<< Updated upstream
	for peer in "${peers[@]}"; do
		if [[ "$peer" =~ ^filestore-peer-[0-9]+$ ]]; then
			echo "Deleting $peer from $network"
			gcloud --project "${PROJECT_ID}" compute networks peerings delete --network "$network" "$peer"
=======
>>>>>>> Stashed changes
		create_time_seconds=$(date -d "$create_time" +%s 2>/dev/null || echo 0)
		age_seconds=$((CURRENT_TIME - create_time_seconds))
		age_hours=$((age_seconds / 3600))

		echo "Evaluating Filestore instance: ${instance} (location: ${location}, created: ${create_time}, age: ${age_hours}h)"

		# Rule 1: Delete immediately if 24 hours or older.
		if ((age_seconds >= CLEANUP_AGE_SECONDS)); then
			echo "Instance ${instance} is 24 hours or older."
			if [ "$DRY_RUN" = "true" ]; then
				echo "[DRY-RUN] Would delete ${instance} at ${location}."
				continue
			else
				echo "Deleting abandoned Filestore instance ${instance}..."
			fi

		# Rule 2: If younger than 24 hours, check whether any Filestore tests are actively running.
		else
			echo "Instance ${instance} is less than 24 hours old. Checking for active Filestore Cloud Builds..."

<<<<<<< Updated upstream
			active_builds=$(gcloud builds list \
				--project "${PROJECT_ID}" \
				--filter="tags=m.filestore" \
				--format="value(id)" \
				--ongoing 2>/dev/null || true)

			if [[ -n "$active_builds" ]]; then
=======
			if [[ -n "$ACTIVE_BUILDS" ]]; then
>>>>>>> Stashed changes
				echo "Active Filestore Cloud Build found (${active_builds}). Keeping ${instance}."
				KEPT_ACTIVE_INSTANCE=true
				continue
			else
				echo "No active Filestore Cloud Builds found."
				if [ "$DRY_RUN" = "true" ]; then
					echo "[DRY-RUN] Would delete ${instance} at ${location} (no active build)."
					continue
				else
					echo "Deleting leaked Filestore instance ${instance}..."
				fi
			fi
<<<<<<< Updated upstream
=======
>>>>>>> Stashed changes
>>>>>>> Stashed changes
		fi

<<<<<<< Updated upstream
=======
<<<<<<< Updated upstream
=======
>>>>>>> Stashed changes
		# Disable deletion protection before deleting the instance.
		echo "Disabling deletion protection for ${instance} at ${location}..."
		gcloud --project "${PROJECT_ID}" \
			filestore instances update "${instance}" \
			--location="${location}" \
			--no-deletion-protection \
			--quiet || true

		echo "Deleting ${instance} at ${location}..."
		gcloud --project "${PROJECT_ID}" \
			filestore instances delete \
			--force \
			--quiet \
			--location="${location}" \
			"${instance}"

		echo "Successfully deleted ${instance}."
	done <<<"$FILESTORE_INSTANCES"
else
	echo "No Filestore instances found in project."
fi

# Step 2: Reset internal Filestore API limits if no tests or active instances are running.
# Toggling file.googleapis.com off and back on clears stuck producer quota and peering state.
echo "Checking if Filestore API internal limits can be reset..."

<<<<<<< Updated upstream
active_builds=$(gcloud builds list \
	--project "${PROJECT_ID}" \
	--filter="tags=m.filestore" \
	--format="value(id)" \
	--ongoing 2>/dev/null || true)

if [[ -n "$active_builds" ]] || [ "$KEPT_ACTIVE_INSTANCE" = true ]; then
=======

if [[ -n "$ACTIVE_BUILDS" ]] || [ "$KEPT_ACTIVE_INSTANCE" = true ]; then
>>>>>>> Stashed changes
	echo "Active Filestore test or instance detected. Skipping API reset to protect active tests."
elif [ "$DRY_RUN" = "true" ]; then
	echo "[DRY-RUN] Would disable file.googleapis.com, sleep 120s, and re-enable it on EXIT."
else
	echo "Disabling Filestore API to reset internal limits..."
	trap enable_filestore_api EXIT
	gcloud services disable file.googleapis.com --force --project "${PROJECT_ID}"
	echo "Sleeping for 2 minutes for internal limits to fully reset..."
	sleep 120
fi

# Step 3: Clean up dangling Filestore VPC network peerings (filestore-peer-*).
# Peerings >= 24 hours old are deleted immediately; younger peerings are only deleted
# when no active Filestore Cloud Builds or active instances exist.
echo "Checking network peerings..."

# Flatten nested peerings list so each line outputs: <peering_name> <network_name>
peerings=$(gcloud compute networks peerings list \
	--project "${PROJECT_ID}" \
	--flatten="peerings[]" \
	--format="value(peerings.name,name)" 2>/dev/null || true)

found_filestore_peerings=false

if [[ -n "$peerings" ]]; then
	while read -r peering network; do
		[[ -z "$peering" ]] && continue

		# Only target Filestore-created peerings.
		if [[ "$peering" =~ ^filestore-peer-[0-9]+$ ]]; then
			found_filestore_peerings=true
			echo "Evaluating peering: ${peering} on network: ${network}"

			# Look up when the peering was created from Cloud Audit Logs.
			creation_time=$(gcloud logging read \
<<<<<<< Updated upstream
				"protoPayload.methodName=~\"compute.networks.addPeering\" AND protoPayload.request.networkPeering.name=\"${peering}\"" \
=======
				"protoPayload.methodName=~\"compute.networks.addPeering\" AND protoPayload.request.networkPeering.name=\"${peering}\" AND timestamp >= \"-P7D\""\
>>>>>>> Stashed changes
				--project="${PROJECT_ID}" \
				--format="value(timestamp)" \
				--limit=1 2>/dev/null || true)

			# If audit logs do not have the creation timestamp, keep the peering if an active instance exists.
			if [[ -z "$creation_time" ]]; then
				echo "Creation time for ${peering} could not be determined."
				if [ "$KEPT_ACTIVE_INSTANCE" = true ]; then
					echo "Keeping peering ${peering} for safety (active instance detected)."
					continue
				fi
			fi

			creation_seconds=$(date -d "$creation_time" +%s 2>/dev/null || echo "")

			if [[ -n "$creation_seconds" ]]; then
				age_seconds=$((CURRENT_TIME - creation_seconds))
				age_hours=$((age_seconds / 3600))
				echo "Peering ${peering} created at ${creation_time} (age: ${age_hours}h)"
			else
				age_seconds=-1
				echo "Peering ${peering} creation time is unknown."
			fi

			# Rule 1: Delete immediately if 24 hours or older.
			if ((age_seconds >= CLEANUP_AGE_SECONDS)); then
				echo "Peering ${peering} is 24 hours or older."
				if [ "$DRY_RUN" = "true" ]; then
					echo "[DRY-RUN] Would delete dangling peering ${peering} from ${network}."
					continue
				else
					echo "Deleting dangling peering ${peering} from ${network}..."
				fi

			# Rule 2: If younger than 24 hours (or unknown age), check for active test builds.
			else
				echo "Peering ${peering} is less than 24 hours old (or unknown age). Checking for active Filestore Cloud Builds..."

<<<<<<< Updated upstream
				active_builds=$(gcloud builds list \
					--project "${PROJECT_ID}" \
					--filter="tags=m.filestore" \
					--format="value(id)" \
					--ongoing 2>/dev/null || true)

				if [[ -n "$active_builds" ]] || [ "$KEPT_ACTIVE_INSTANCE" = true ]; then
=======
				if [[ -n "$ACTIVE_BUILDS" ]] || [ "$KEPT_ACTIVE_INSTANCE" = true ]; then
>>>>>>> Stashed changes
					echo "Active Filestore Cloud Build or instance found. Keeping peering ${peering}."
					continue
				else
					echo "No active Filestore Cloud Builds found."
					if [ "$DRY_RUN" = "true" ]; then
						echo "[DRY-RUN] Would delete peering ${peering} from ${network} (no active build)."
						continue
					else
						echo "Deleting dangling peering ${peering} from ${network}..."
					fi
				fi
			fi

			gcloud compute networks peerings delete \
				--project "${PROJECT_ID}" \
				--network "${network}" \
				"${peering}" \
				--quiet || true

			echo "Successfully deleted peering ${peering}."
		fi
	done <<<"$peerings"
fi

if [ "$found_filestore_peerings" = false ]; then
	echo "No dangling filestore-peer-* connections found in project."
fi

echo "Filestore & Peering cleanup completed successfully."
<<<<<<< Updated upstream
=======
>>>>>>> Stashed changes
>>>>>>> Stashed changes
exit 0
