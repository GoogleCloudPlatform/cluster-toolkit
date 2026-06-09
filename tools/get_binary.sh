#!/bin/bash
# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -x -e -o pipefail

TEST_PREFIX=${1:-""}
BUILD_FROM_SOURCE=${2:-"false"}

if [ "${TEST_PREFIX}" == "daily-" ] && [ "${BUILD_FROM_SOURCE}" != "true" ]; then
	if [ -z "${GCLUSTER_GCS_PATH}" ]; then
		echo "Error: GCLUSTER_GCS_PATH environment variable is not set." >&2
		exit 1
	fi
	gsutil cp "gs://${GCLUSTER_GCS_PATH}/latest/gcluster-bundle.zip" .
	unzip -o gcluster-bundle.zip >/dev/null
	unzip -l gcluster-bundle.zip | tail -n 1 | awk '{print "Extracted " $2 " " $3 " (Total size: " $1 " bytes)."}'
	# Grant execution permissions to the binary
	chmod +x gcluster
else
	make
fi
