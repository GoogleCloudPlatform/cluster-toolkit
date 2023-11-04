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

ACTIVE_FILESTORE=$(gcloud filestore instances list --project "${PROJECT_ID}" | tail -n +2 2>/dev/null)
if [[ -n "$ACTIVE_FILESTORE" ]]; then
	echo "Deleting filestore instances"
	while read -r row; do
		# get first two columns: INSTANCE_NAME and LOCATION
		read -ra cols <<<"$row"
		echo "Deleting ${cols[0]} at ${cols[0]}"
		gcloud --project "${PROJECT_ID}" filestore instances delete --force --quiet --location="${cols[1]}" "${cols[0]}"
	done <<<"$ACTIVE_FILESTORE"
fi

# See https://cloud.google.com/filestore/docs/troubleshooting#system_limit_for_internal_resources_has_been_reached_error_when_creating_an_instance
echo "Disabling Filestore API..."
gcloud services disable file.googleapis.com --force --project "${PROJECT_ID}"

echo "Deleting all Filestore peering networks"
# the output of this command matches
# filestore-peer-426414172628;filestore-peer-646290499454 default
peerings=$(gcloud compute networks peerings list --project "${PROJECT_ID}" --format="value(peerings.name,name)")
while read -r peering; do
	# split the output into:
	# 0: a semi-colon separated list of peerings
	# 1: the name of a VPC network
	read -ra parr <<<"$peering"
	# split the list of peerings into an array
	IFS=";" read -ra peers <<<"${parr[0]}"
	# capture the VPC network
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

exit 0
