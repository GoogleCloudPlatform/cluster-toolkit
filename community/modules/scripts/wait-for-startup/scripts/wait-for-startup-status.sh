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
# Match string for all finish types of the old guest agent and successful
# finishes on the new guest agent
FINISH_LINE="startup-script exit status"
# Match string for failures on the new guest agent
FINISH_LINE_ERR="Script.*failed with error:"

NON_FATAL_ERRORS=(
	"Internal error"
)

until [[ now -gt deadline ]]; do
	ser_log=$(
		set -o pipefail
		${fetch_cmd} 2>"${error_file}" |
			c1grep "${FINISH_LINE}\|${FINISH_LINE_ERR}"
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
			exit 1
		fi
	}
	if [[ -n "${ser_log}" ]]; then break; fi
	echo "Could not detect end of startup script. Sleeping."
	sleep 5
	now=$(date +%s)
done

# This line checks for an exit code - the assumption is that there is a number
# at the end of the line and it is an exit code
STATUS=$(sed -r 's/.*([0-9]+)\s*$/\1/' <<<"${ser_log}" | uniq)
# This specific text is monitored for in tests, do not change.
INSPECT_OUTPUT_TEXT="To inspect the startup script output, please run:"
if [[ "${STATUS}" == 0 ]]; then
	echo "startup-script finished successfully"
elif [[ "${STATUS}" == 1 ]]; then
	echo "startup-script finished with errors, ${INSPECT_OUTPUT_TEXT}"
	echo "${fetch_cmd}"
elif [[ now -ge deadline ]]; then
	echo "startup-script timed out after ${TIMEOUT} seconds"
	echo "${INSPECT_OUTPUT_TEXT}"
	echo "${fetch_cmd}"
	exit 1
else
	echo "Invalid return status: '${STATUS}'"
	echo "${INSPECT_OUTPUT_TEXT}"
	echo "${fetch_cmd}"
	exit 1
fi

exit "${STATUS}"
