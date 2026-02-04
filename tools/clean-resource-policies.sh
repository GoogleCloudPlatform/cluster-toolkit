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

#

set -e

PROJECT_ID=${PROJECT_ID:-$(gcloud config get-value project)}
if [ -z "$PROJECT_ID" ]; then
	echo "PROJECT_ID must be defined"
	exit 1
fi
echo "processing resource policies for $PROJECT_ID"
# expecting results in the following format:
# <group name>, https://www.googleapis.com/compute/v1/projects/<project name>/regions/<region>
REGEX="(.*),.*\/([a-z0-9-]+)"

declare -A policies
for line in $(gcloud compute resource-policies list --format='csv[no-heading](name, region)' --project "${PROJECT_ID}"); do
	if [[ $line =~ $REGEX ]]; then
		policy=${BASH_REMATCH[1]}
		region=${BASH_REMATCH[2]}
		policies[$policy]=$region
		echo "found ${policy} in ${region}"
	else
		echo "not a match: $line"
	fi
done

for policy in "${!policies[@]}"; do
	gcloud compute resource-policies delete "$policy" \
		--project "${PROJECT_ID}" --region "${policies[$policy]}" || true
done
