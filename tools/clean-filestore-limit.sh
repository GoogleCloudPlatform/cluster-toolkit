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

# Unlabeled Filestore instances older than 24 hours are considered orphaned.
CLEANUP_AGE_SECONDS=$((24 * 60 * 60))
CURRENT_TIME=$(date -u +%s)
KEPT_ACTIVE_INSTANCE=false
CLEANUP_FAILED=false

echo "Starting Filestore & Peering Cleanup for project: ${PROJECT_ID} (DRY_RUN=${DRY_RUN})"
if [ "$DRY_RUN" = "true" ]; then
	echo "SIMULATION ONLY: No resources will actually be deleted."
fi

# Step 1: Clean up Filestore instances using time-to-live (TTL) labels and 24h fallback.
# - TTL not expired, or unparsable -> in use -> keep instance and skip API reset.
# - TTL expired -> leftover -> delete.
# - No TTL label and createTime >= 24h -> leftover -> delete.
# - No TTL label and createTime < 24h (or unparsable) -> in use -> keep instance and skip API reset.
if ! FILESTORE_INSTANCES=$(gcloud filestore instances list \
	--project "${PROJECT_ID}" \
	--format="value(name.basename(),name.segment(3),createTime,labels.time-to-live)"); then
	echo "ERROR: Failed to list Filestore instances; not touching anything." >&2
	exit 1
fi

if [[ -n "$FILESTORE_INSTANCES" ]]; then
	while read -r instance location create_time ttl_label; do
		[[ -z "$instance" ]] && continue

		echo "Evaluating Filestore instance: ${instance} (location: ${location}, created: ${create_time}, ttl: ${ttl_label:-none})"

		# Case 1: Instance has a time-to-live label (expected format: YYYY-MM-DD or YYYY-MM-DD_HH-MM).
		if [[ -n "$ttl_label" ]]; then
			if [[ "$ttl_label" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}(_[0-9]{2}-[0-9]{2})?$ ]]; then
				ttl_for_date=$(echo "$ttl_label" | sed 's/_/ /; s/-/:/3')
				if ! ttl_seconds=$(date -u -d "$ttl_for_date" +%s 2>/dev/null); then
					echo "Unparsable time-to-live label (${ttl_label}) on ${instance}. Keeping instance."
					KEPT_ACTIVE_INSTANCE=true
					continue
				fi

				if ((ttl_seconds > CURRENT_TIME)); then
					echo "TTL label (${ttl_label}) on ${instance} has not expired. Keeping instance."
					KEPT_ACTIVE_INSTANCE=true
					continue
				fi

				echo "TTL label (${ttl_label}) on ${instance} has expired."
			else
				echo "Invalid time-to-live label format (${ttl_label}) on ${instance}. Keeping instance."
				KEPT_ACTIVE_INSTANCE=true
				continue
			fi

		# Case 2: Instance has no time-to-live label; fall back to 24-hour createTime check.
		else
			if ! create_time_seconds=$(date -u -d "$create_time" +%s 2>/dev/null); then
				echo "Unparsable createTime (${create_time}) on ${instance}. Keeping instance."
				KEPT_ACTIVE_INSTANCE=true
				continue
			fi

			age_seconds=$((CURRENT_TIME - create_time_seconds))
			age_hours=$((age_seconds / 3600))

			if ((age_seconds < CLEANUP_AGE_SECONDS)); then
				echo "Instance ${instance} has no TTL label and is less than 24 hours old (${age_hours}h). Keeping instance."
				KEPT_ACTIVE_INSTANCE=true
				continue
			fi

			echo "Instance ${instance} has no TTL label and is 24 hours or older (${age_hours}h)."
		fi

		if [ "$DRY_RUN" = "true" ]; then
			echo "[DRY-RUN] Would delete ${instance} at ${location}."
			continue
		fi

		# Disable deletion protection before deleting the instance.
		echo "Disabling deletion protection for ${instance} at ${location}..."
		gcloud --project "${PROJECT_ID}" \
			filestore instances update "${instance}" \
			--location="${location}" \
			--no-deletion-protection \
			--quiet || true

		echo "Deleting ${instance} at ${location}..."
		if ! gcloud --project "${PROJECT_ID}" \
			filestore instances delete \
			--force \
			--quiet \
			--location="${location}" \
			"${instance}"; then
			echo "ERROR: Failed to delete ${instance} at ${location}." >&2
			CLEANUP_FAILED=true
			KEPT_ACTIVE_INSTANCE=true
			continue
		fi

		echo "Successfully deleted ${instance}."
	done <<<"$FILESTORE_INSTANCES"
else
	echo "No Filestore instances found in project."
fi

# Step 2: Re-query ongoing Cloud Builds immediately before resetting the Filestore API.
# Failing closed ensures a transient gcloud error or newly started build is never disrupted.
echo "Checking for active Filestore Cloud Builds before resetting Filestore API..."

if ! CURRENT_BUILDS=$(gcloud builds list \
	--project "${PROJECT_ID}" \
	--filter="tags=m.filestore" \
	--format="value(id)" \
	--ongoing); then
	echo "ERROR: Failed to list ongoing Cloud Builds; not resetting." >&2
	exit 1
fi

if [[ -n "$CURRENT_BUILDS" ]] || [ "$KEPT_ACTIVE_INSTANCE" = true ]; then
	echo "Filestore is in use (active builds: ${CURRENT_BUILDS:-none}, kept instance: ${KEPT_ACTIVE_INSTANCE}); skipping API reset and peering deletion."
	[ "$CLEANUP_FAILED" = true ] && exit 1
	echo "Filestore & Peering cleanup completed successfully."
	exit 0
fi

if [ "$DRY_RUN" = "true" ]; then
	echo "[DRY-RUN] Would disable file.googleapis.com, sleep 120s, and re-enable it on EXIT."
else
	echo "Disabling Filestore API to reset internal limits..."
	trap enable_filestore_api EXIT
	gcloud services disable file.googleapis.com --force --project "${PROJECT_ID}"
	echo "Sleeping for 2 minutes for internal limits to fully reset..."
	sleep 120
fi

# Step 3: Delete all Filestore VPC network peerings (safe because no instances or active tests exist).
echo "Deleting Filestore peering networks..."
if ! PEERINGS=$(gcloud compute networks peerings list \
	--project "${PROJECT_ID}" \
	--flatten="peerings[]" \
	--format="value(peerings.name,name)"); then
	echo "ERROR: Failed to list network peerings." >&2
	CLEANUP_FAILED=true
fi

if [[ -n "$PEERINGS" ]]; then
	while read -r peering network; do
		[[ "$peering" =~ ^filestore-peer-[0-9]+$ ]] || continue
		if [ "$DRY_RUN" = "true" ]; then
			echo "[DRY-RUN] Would delete peering ${peering} from ${network}."
			continue
		fi
		echo "Deleting ${peering} from ${network}..."
		if ! gcloud compute networks peerings delete "$peering" \
			--network "$network" \
			--project "${PROJECT_ID}" \
			--quiet; then
			echo "ERROR: Failed to delete peering ${peering} from ${network}." >&2
			CLEANUP_FAILED=true
		fi
	done <<<"$PEERINGS"
else
	echo "No network peerings found."
fi

[ "$CLEANUP_FAILED" = true ] && exit 1
echo "Filestore & Peering cleanup completed successfully."
exit 0
