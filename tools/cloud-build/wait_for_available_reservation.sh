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
PROJECT=$3
REQUIRED_COUNT=${4:-1}

if [[ -z "${RESERVATION}" || -z "${ZONE}" ]]; then
	echo "Usage: $0 <RESERVATION_NAME> <ZONE> [PROJECT_ID] [REQUIRED_COUNT]"
	exit 1
fi

if [[ ! "${REQUIRED_COUNT}" =~ ^[1-9][0-9]*$ ]]; then
	echo "Error: REQUIRED_COUNT must be a positive integer." >&2
	exit 1
fi

PROJECT_ARGS=()
if [[ -n "${PROJECT}" ]]; then
	PROJECT_ARGS=("--project=${PROJECT}")
fi

RESERVATION_OUTPUT=$(mktemp) || {
	echo "Failed to create temp file" >&2
	exit 1
}
trap 'rm -f "${RESERVATION_OUTPUT}"' EXIT

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
			--format="value(specificReservation.count, specificReservation.inUseCount)"); then

			echo "Failed to query reservation details."
			exit 1
		fi

		read -r TOTAL_COUNT IN_USE_COUNT <<<"${OUTPUT}"
		IN_USE_COUNT=${IN_USE_COUNT:-0}

		if [[ ! "${TOTAL_COUNT}" =~ ^[0-9]+$ ]] || [[ ! "${IN_USE_COUNT}" =~ ^[0-9]+$ ]]; then
			echo "Failed to parse reservation capacity from gcloud output."
			exit 1
		fi

		AVAILABLE=$((TOTAL_COUNT - IN_USE_COUNT))
		echo "Reservation '${RESERVATION}' capacity: Total=${TOTAL_COUNT}, InUse=${IN_USE_COUNT}, Available=${AVAILABLE}"

		if [[ "${TOTAL_COUNT}" -eq 0 ]]; then
			echo "Reservation has 0 total capacity."
			exit 3
		fi
		if [[ ${AVAILABLE} -ge ${REQUIRED_COUNT} ]]; then
			exit 0
		else
			echo "Reservation is currently fully in use"
			exit 2
		fi
	) 2>&1 | tee "$RESERVATION_OUTPUT"

	EXIT_CODE=${PIPESTATUS[0]}

	if [ "$EXIT_CODE" -eq 0 ]; then
		echo "--- SUCCESS: Reservation slot is available. ---"
		break
	elif [ "$EXIT_CODE" -eq 3 ]; then
		echo "--- FATAL ERROR: Reservation has 0 total capacity. Exiting. ---" >&2
		exit 1
	else
		# Check if the failure was specifically due to reservation capacity
		if grep -q "Reservation is currently fully in use" "$RESERVATION_OUTPUT"; then
			echo "--- RETRYING in 5 minutes... ---" >&2
			sleep 300
		elif grep -qiE "not found|permission|denied|invalid|required" "$RESERVATION_OUTPUT"; then
			echo "--- FATAL ERROR: Checking reservation failed due to a configuration error. Exiting. ---" >&2
			exit 1
		else
			echo "--- WARNING: Transient or unexpected error encountered. Retrying in 1 minute... ---" >&2
			sleep 60
		fi
	fi
done
