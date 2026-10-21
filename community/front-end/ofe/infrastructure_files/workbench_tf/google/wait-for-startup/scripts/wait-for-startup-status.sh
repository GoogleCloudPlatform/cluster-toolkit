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

# Readiness detection for Vertex AI Workbench Instances.
#
# Polls the Workbench API for `state == ACTIVE` (JupyterLab up, proxy URI
# populated) rather than a serial-console marker, which Workbench images
# never emit. This is independent of the OFE post-startup script, so a
# slow or failing post-startup step does not block or fail deployment.

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
export CLOUDSDK_CONFIG
GOOGLE_APPLICATION_CREDENTIALS="../../cloud_credentials"
gcloud auth activate-service-account --key-file="${GOOGLE_APPLICATION_CREDENTIALS}"

# TIMEOUT: 0 = ready, 1 = timed out, 2 = instance entered a terminal
# non-ready state (stopped/suspended/deleted) during bring-up.
TIMEOUT=1
tries=0
until [ "$tries" -ge "${RETRIES}" ]; do
	STATE=$(gcloud workbench instances describe "${INSTANCE_NAME}" \
		--location "${ZONE}" --project "${PROJECT_ID}" \
		--format="value(state)" 2>/dev/null)
	echo "workbench state: '${STATE}'"
	if [ "${STATE}" == "ACTIVE" ]; then
		TIMEOUT=0
		break
	fi
	case "${STATE}" in
	STOPPED | SUSPENDED | DELETED)
		TIMEOUT=2
		break
		;;
	esac
	echo "workbench not ACTIVE yet. Sleeping."
	sleep 30
	((tries++))
done

rm -r "${CLOUDSDK_CONFIG}"

if [ "${TIMEOUT}" -eq "0" ]; then
	echo "Workbench instance is ACTIVE"
elif [ "${TIMEOUT}" -eq "1" ]; then
	echo "Timed out waiting for workbench instance to become ACTIVE" >&2
else
	echo "Workbench instance entered a terminal non-ready state: '${STATE}'" >&2
fi

exit "${TIMEOUT}"
