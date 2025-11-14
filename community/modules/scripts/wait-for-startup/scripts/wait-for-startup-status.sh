#!/bin/bash
# Copyright 2022 Google LLC
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

if [[ -z "${INSTANCE_NAME}" ]]; then
	echo "INSTANCE_NAME is unset... exiting"
	exit 0
fi
if [[ -z "${ZONE}" ]]; then
	echo "ZONE is unset"
	exit 1
fi
if [[ -z "${PROJECT_ID}" ]]; then
	echo "PROJECT_ID is unset"
	exit 1
fi
if [[ -z "${TIMEOUT}" ]]; then
	echo "TIMEOUT is unset"
	exit 1
fi

if [[ -n "${GCLOUD_PATH}" ]]; then
	export PATH="$GCLOUD_PATH:$PATH"
fi

echo "Waiting for startup: instance_name='${INSTANCE_NAME}', zone='${ZONE}', project_id='${PROJECT_ID}', timeout_seconds='${TIMEOUT}'"

# Wrapper around grep that swallows the error status code 1
c1grep() { grep "$@" || test $? = 1; }

now=$(date +%s)

# If VM was created more than 30 days ago, serial port logs may no longer exist.
# Exit without errors if the instance is older than 30 days.
logsExpiryDays=30
createdTimestampIso=$(gcloud compute instances describe "${INSTANCE_NAME}" --project "${PROJECT_ID}" --zone "${ZONE}" --format "value(creationTimestamp)")
earliestAllowedCreatedTimestamp=$(date -d "${createdTimestampIso} +${logsExpiryDays} day" +%s)
if [[ "$earliestAllowedCreatedTimestamp" -lt "$now" ]]; then
	echo "Instance was created more than 30 days ago - serial port 1 logs are likely expired... exiting"
	exit 0
fi

deadline=$((now + TIMEOUT))
error_file=$(mktemp)
fetch_cmd="gcloud compute instances get-serial-port-output ${INSTANCE_NAME} --port 1 --zone ${ZONE} --project ${PROJECT_ID}"

# Custom finish line pattern
FINISH_LINE_CUSTOM="passed_startup_script.sh-.* finished with exit_code="

# Original patterns for compatibility or other script types
FINISH_LINE="startup-script exit status"
FINISH_LINE_ERR="Script.*failed with error:"

NON_FATAL_ERRORS=(
	"Internal error"
)

ser_log=""
until [[ $(date +%s) -gt deadline ]]; do
	current_ser_log=$(
		set -o pipefail
		${fetch_cmd} 2>"${error_file}" |
			c1grep -E "${FINISH_LINE_CUSTOM}|${FINISH_LINE}|${FINISH_LINE_ERR}"
	) || {
		err=$(cat "${error_file}")
		echo "$err"
		fatal_error="true"
		for e in "${NON_FATAL_ERRORS[@]}"; do
			if [[ $err = *"$e"* ]]; then
				fatal_error="false"
				break
			fi
		done

		if [[ $fatal_error = "true" ]]; then
			rm -f "${error_file}"
			exit 1
		fi
	}
	if [[ -n "${current_ser_log}" ]]; then
		ser_log="${current_ser_log}"
		break
	fi
	sleep 5
done
rm -f "${error_file}"

INSPECT_OUTPUT_TEXT="To inspect the startup script output, please run:"

if [[ -z "${ser_log}" ]]; then
	# This case means timeout
	echo "startup-script timed out after ${TIMEOUT} seconds"
	echo "${INSPECT_OUTPUT_TEXT}"
	echo "${fetch_cmd}"
	exit 1
fi

STATUS=""
EXIT_CODE_FOUND=false

if echo "${ser_log}" | grep -qE "${FINISH_LINE_CUSTOM}"; then
	STATUS=$(echo "${ser_log}" | sed -rn "s/.*${FINISH_LINE_CUSTOM}([0-9]+).*/\1/p" | head -n 1)
	if [[ "${STATUS}" =~ ^[0-9]+$ ]]; then
		echo "Detected custom finish pattern. Startup script exit code: ${STATUS}"
		EXIT_CODE_FOUND=true
	else
		echo "Warning: Matched custom finish pattern but failed to extract exit code from: ${ser_log}"
		STATUS=1 # Fallback to general error
	fi
elif echo "${ser_log}" | grep -qE "${FINISH_LINE}"; then
	TEMP_STATUS=$(sed -r 's/.*([0-9]+)\s*$/\1/' <<<"${ser_log}" | uniq | head -n 1)
	if [[ "${TEMP_STATUS}" =~ ^[0-9]+$ ]]; then
		STATUS=${TEMP_STATUS}
		echo "Detected old finish pattern. Startup script exit code: ${STATUS}"
		EXIT_CODE_FOUND=true
	else
		echo "Warning: Matched old finish pattern but failed to extract exit code from: ${ser_log}"
		STATUS=1 # Fallback to general error
	fi
elif echo "${ser_log}" | grep -qE "${FINISH_LINE_ERR}"; then
	echo "startup-script failed (error line detected): ${ser_log}"
	STATUS=1 # General error
else
	echo "Unknown completion log line: ${ser_log}"
	echo "${INSPECT_OUTPUT_TEXT}"
	echo "${fetch_cmd}"
	exit 1
fi

if [[ "${EXIT_CODE_FOUND}" == "true" ]]; then
	if [[ "${STATUS}" == 0 ]]; then
		echo "Startup script completed successfully."
	else
		echo "Startup script completed with errors (exit code ${STATUS})."
		echo "${INSPECT_OUTPUT_TEXT}"
		echo "${fetch_cmd}"
	fi
	exit "${STATUS}"
else
	# Fallback for cases where a pattern matched but code extraction failed, or FINISH_LINE_ERR
	echo "Startup script finished with errors (assumed exit code ${STATUS})."
	echo "${INSPECT_OUTPUT_TEXT}"
	echo "${fetch_cmd}"
	exit "${STATUS}"
fi
