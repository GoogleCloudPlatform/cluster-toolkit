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

if [ -z "${INSTANCE_NAME}" ]; then
	echo "INSTANCE_NAME is unset"
	exit 1
fi
if [ -z "${ZONE}" ]; then
	echo "ZONE is unset"
	exit 1
fi
if [ -z "${PROJECT_ID}" ]; then
	echo "PROJECT_ID is unset"
	exit 1
fi

tries=0
until [ $tries -ge "${RETRIES}" ]; do
	GCLOUD="gcloud compute instances get-serial-port-output ${INSTANCE_NAME} --port 1 --zone ${ZONE} --project ${PROJECT_ID}"
	FINISH_LINE="startup-script exit status"
	STATUS_LINE=$(${GCLOUD} 2>/dev/null | grep "${FINISH_LINE}")
	STATUS=$(sed -r 's/.*([0-9]+)\s*$/\1/' <<<"${STATUS_LINE}" | uniq)
	if [ -n "${STATUS}" ]; then break; fi
	echo "could not detect end of startup script. Sleeping."
	sleep 5
	((tries++))
done

# This specific text is monitored for in tests, do not change.
INSPECT_OUTPUT_TEXT="to inspect the startup script output, please run:"
if [ "${STATUS}" == 0 ]; then
	echo "startup-script finished successfully"
elif [ "${STATUS}" == 1 ]; then
	echo "startup-script finished with errors, ${INSPECT_OUTPUT_TEXT}"
	echo "${GCLOUD}"
else
	echo "invalid return status '${STATUS}'"
	echo "${INSPECT_OUTPUT_TEXT}"
	echo "${GCLOUD}"
	exit 1
fi

exit "${STATUS}"
