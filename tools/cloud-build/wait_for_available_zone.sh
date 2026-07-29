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

# Wrapper around find_available_zone.sh that loops instead of exiting when out of capacity.
# Since find_available_zone.sh uses 'exit 1', we must run it in a subshell
# to prevent it from killing the pod.
ZONE_EXPORT=$(mktemp)
ZONE_OUTPUT=$(mktemp)
trap 'rm -f "$ZONE_EXPORT" "$ZONE_OUTPUT"' EXIT

while true; do
	# Run the script in a subshell, streaming stdout and stderr to the console and a log file.
	# To extract the exported variables, we have the subshell write them to a file.
	set +e
	(
		source /workspace/tools/cloud-build/find_available_zone.sh
		# If it succeeds, these lines will execute and save the exports.
		echo "export ZONE=\"${ZONE}\"" >"$ZONE_EXPORT"
		echo "export PROVISIONING_MODEL=\"${PROVISIONING_MODEL}\"" >>"$ZONE_EXPORT"
	) 2>&1 | tee "$ZONE_OUTPUT"

	EXIT_CODE=${PIPESTATUS[0]}
	set -e

	if [ "$EXIT_CODE" -eq 0 ]; then
		# shellcheck source=/dev/null
		source "$ZONE_EXPORT"
		break
	else
		# Check if the failure was specifically due to zone capacity
		if grep -q "Couldn't find a zone to deploy" "$ZONE_OUTPUT"; then
			echo "--- RETRYING in 5 minutes... ---" >&2
			sleep 300
		else
			echo "--- FATAL ERROR: find_available_zone.sh failed due to a configuration or system error. Exiting. ---" >&2
			exit 1
		fi
	fi
done

rm -f "$ZONE_EXPORT" "$ZONE_OUTPUT"
trap - EXIT
