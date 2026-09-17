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

# wait_for_available_reservation.sh
# Polls a GCE reservation to check for available capacity.
# If the reservation lacks sufficient capacity for the requested nodes, it enters a polling loop.

RESERVATION=$1
ZONE=$2
RESERVATION_PROJECT=$3
REQUIRED_COUNT=${4:-1}

# If the reservation is provided as a fully qualified URI (projects/<PROJECT>/...),
# extract the owner project automatically.
if [[ "${RESERVATION}" =~ ^projects/([^/]+)/ ]]; then
	RESERVATION_PROJECT="${BASH_REMATCH[1]}"
fi
RESERVATION="${RESERVATION##*/}"
if [[ -z "${RESERVATION}" || -z "${ZONE}" ]]; then
	echo "Usage: $0 <RESERVATION_NAME|RESOURCE_URI> <ZONE> [RESERVATION_OWNER_PROJECT] [REQUIRED_COUNT]" >&2
	exit 1
fi
if [[ ! "${REQUIRED_COUNT}" =~ ^[1-9][0-9]*$ ]]; then
	echo "Error: REQUIRED_COUNT must be a positive integer." >&2
	exit 1
fi
PROJECT_ARGS=()
if [[ -n "${RESERVATION_PROJECT}" ]]; then
	PROJECT_ARGS=("--project=${RESERVATION_PROJECT}")
fi

RESERVATION_OUTPUT=$(mktemp) || {
	echo "Failed to create temp file" >&2
	exit 1
}
trap 'rm -f "${RESERVATION_OUTPUT}"' EXIT SIGTERM SIGINT

MAX_WAIT_SECONDS=28800 # 8 hours
START_TIME=$SECONDS

while true; do
	if [ $((SECONDS - START_TIME)) -gt $MAX_WAIT_SECONDS ]; then
		echo "--- FATAL ERROR: Timed out waiting for available reservation after 8 hours. Exiting. ---" >&2
		exit 1
	fi

	# Run the capacity check in a subshell, streaming stdout and stderr to the console and a log file.
	(
		if ! OUTPUT=$(gcloud compute reservations describe "${RESERVATION}" \
			--zone="${ZONE}" \
			"${PROJECT_ARGS[@]}" \
			--format="value(status, specificReservation.count, specificReservation.inUseCount)"); then

			echo "Failed to query reservation details from gcloud." >&2
			exit 1
		fi

		read -r RES_STATUS TOTAL_COUNT IN_USE_COUNT <<<"${OUTPUT}"
		IN_USE_COUNT=${IN_USE_COUNT:-0}

		if [[ "${RES_STATUS}" != "READY" ]]; then
			echo "Reservation '${RESERVATION}' is not in READY state (current status: ${RES_STATUS:-UNKNOWN})." >&2
			exit 3
		fi

		if [[ ! "${TOTAL_COUNT}" =~ ^[0-9]+$ ]] || [[ ! "${IN_USE_COUNT}" =~ ^[0-9]+$ ]]; then
			echo "Failed to parse numeric capacity from gcloud output: '${OUTPUT}'" >&2
			exit 3
		fi

		AVAILABLE=$((TOTAL_COUNT - IN_USE_COUNT))
		echo "Reservation '${RESERVATION}' capacity: Total=${TOTAL_COUNT}, InUse=${IN_USE_COUNT}, Available=${AVAILABLE}"

		if [[ "${TOTAL_COUNT}" -eq 0 ]]; then
			echo "Reservation '${RESERVATION}' has 0 total capacity." >&2
			exit 3
		fi
		if [[ "${REQUIRED_COUNT}" -gt "${TOTAL_COUNT}" ]]; then
			echo "Reservation '${RESERVATION}' total capacity (${TOTAL_COUNT}) is less than required count (${REQUIRED_COUNT})." >&2
			exit 3
		fi
		if [[ ${AVAILABLE} -ge ${REQUIRED_COUNT} ]]; then
			exit 0
		else
			echo "Insufficient capacity in reservation '${RESERVATION}' (${AVAILABLE} available, ${REQUIRED_COUNT} required)."
			exit 2
		fi
	) 2>&1 | tee "$RESERVATION_OUTPUT"

	EXIT_CODE=${PIPESTATUS[0]}

	if [ "$EXIT_CODE" -eq 0 ]; then
		echo "--- SUCCESS: Reservation slot is available. ---"
		break
	elif [ "$EXIT_CODE" -eq 2 ]; then
		echo "--- Insufficient capacity. RETRYING in 5 minutes... ---" >&2
		sleep 300
	elif [ "$EXIT_CODE" -eq 3 ]; then
		echo "--- FATAL ERROR: Reservation configuration or state error. Exiting. ---" >&2
		exit 3
	else
		# EXIT_CODE=1 (gcloud query failure)
		if grep -qiE "HTTPError (400|403|404)|not[-_ ]found|permission[-_ ]denied|forbidden" "$RESERVATION_OUTPUT"; then
			echo "--- FATAL ERROR: Reservation query failed due to permission or non-existent resource. Exiting. ---" >&2
			exit 1
		else
			echo "--- WARNING: Transient gcloud error encountered. Retrying in 1 minute... ---" >&2
			sleep 60
		fi
	fi
done
