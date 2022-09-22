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

set -e -o pipefail

BUILD_ID=${BUILD_ID:-non-existent-build}
PROJECT_ID=${PROJECT_ID:-$(gcloud config get-value project)}
if [ -z "$PROJECT_ID" ]; then
	echo "PROJECT_ID must be defined"
	exit 1
fi

ACTIVE_BUILDS=$(gcloud builds list --project "${PROJECT_ID}" --filter="id!=\"${BUILD_ID}\"" --ongoing 2>/dev/null)
ACTIVE_FILESTORE=$(gcloud filestore instances list --project "${PROJECT_ID}" --format='value(name)')
if [[ -z "$ACTIVE_BUILDS" && -z "$ACTIVE_FILESTORE" ]]; then
	echo "Disabling Filestore API..."
	gcloud services disable file.googleapis.com --force --project "${PROJECT_ID}"

	echo "Deleting all Filestore peering networks"
	peerings=$(gcloud compute networks peerings list --project "${PROJECT_ID}" --format="value(peerings.name,name)")
	while read -r peering; do
		parr=("$peering")
		IFS=";" read -ra peers <<<"${parr[0]}"
		network=${parr[1]}

		for peer in "${peers[@]}"; do
			if [[ "$peer" =~ ^filestore-peer-[0-9]+$ ]]; then
				echo "Deleting $peer from $network"
				gcloud --project "${PROJECT_ID}" compute networks peerings delete --network "$network" "$peer"
			fi
		done
	done <<<"$peerings"

	echo "Re-enabling Filestore API..."
	gcloud services enable file.googleapis.com --project "${PROJECT_ID}"
	echo "Re-enabled Filestore API..."
elif [[ -n "$ACTIVE_BUILDS" ]]; then
	echo "There are active Cloud Build jobs. Refusing to disable/enable Filestore to reset internal limit."
elif [[ -n "$ACTIVE_FILESTORE" ]]; then
	echo "There are active Filestore instances. These may require manual cleanup."
fi

exit 0
