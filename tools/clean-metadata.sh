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

# Script checks for any slurm metadata that does not have at least one
# corresponding VM instance.  We can assume the metadata is orphaned at that
# point and delete it.

set -e

# Check for project id
PROJECT_ID=${PROJECT_ID:-$(gcloud config get-value project)}
if [ -z "$PROJECT_ID" ]; then
	echo "PROJECT_ID must be defined"
	exit 1
fi
echo "Checking for left over compute metadata for $PROJECT_ID"

# Gather gcloud data - specifically checking for keys that match the slurm metadata pattern
KEYS=$(gcloud compute --project "${PROJECT_ID}" project-info describe --flatten="commonInstanceMetadata[]" | grep "key: .*-slurm-.*" | sed 's/.*: //')
INSTANCES=$(gcloud compute instances list)

# Loop through metadata keys and check for keys that don't have corresponding instances running
if [ "${KEYS}" != "" ]; then
	while read -r key; do
		CLUSTER_NAME=${key/-*/}
		if [ "$(echo "${INSTANCES}" | grep "${CLUSTER_NAME}")" == "" ]; then
			echo "Metadata '${key}' has no corresponding cluster, removing."
			if [ -z "${KEYS_TO_DELETE}" ]; then
				KEYS_TO_DELETE="${key}"
			else
				KEYS_TO_DELETE="${KEYS_TO_DELETE},${key}"
			fi
		fi
	done <<<"${KEYS}"
fi

# Delete keys that are orphaned
if [ -v KEYS_TO_DELETE ]; then
	echo "Running gcloud compute project-info remove-metadata --keys=\"${KEYS_TO_DELETE}\""
	gcloud compute project-info remove-metadata --keys="${KEYS_TO_DELETE}"
fi
