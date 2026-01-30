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

CLOUDSDK_CONFIG=$(mktemp -d)
GOOGLE_APPLICATION_CREDENTIALS="../../cloud_credentials"
gcloud auth activate-service-account --key-file=${GOOGLE_APPLICATION_CREDENTIALS}
GCLOUD="gcloud compute instances get-serial-port-output ${INSTANCE_NAME} --port 1 --zone ${ZONE} --project ${PROJECT_ID}"
FINISH_LINE="BRINGUP COMPLETE"
ERROR_LINE="BRINGUP FAILED"

TIMEOUT=1
tries=0
until [ $tries -ge "${RETRIES}" ]; do
	STATUS_LINE=$(${GCLOUD} 2>/dev/null | grep -oE "${FINISH_LINE}|${ERROR_LINE}")
	echo "STATUS_LINE: '${STATUS_LINE}'"
	if [ "${STATUS_LINE}" == "${FINISH_LINE}" ]; then
		TIMEOUT=0
		break
	fi
	if [ "${STATUS_LINE}" == "${ERROR_LINE}" ]; then
		TIMEOUT=2
		break
	fi
	echo "could not detect end of startup script. Sleeping."
	sleep 30
	((tries++))
done

rm -r "${CLOUDSDK_CONFIG}"

if [ "${TIMEOUT}" -eq "0" ]; then
	echo "Startup script completed"
elif [ "${TIMEOUT}" -eq "1" ]; then
	echo "Startup script timed out" >&2
else
	echo "Node bringup failed - check virtual serial console logs" >&2
fi

exit "${TIMEOUT}"
