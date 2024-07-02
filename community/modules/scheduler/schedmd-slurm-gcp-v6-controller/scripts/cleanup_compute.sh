#!/bin/bash

# Copyright 2024 Google LLC
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

project="$1"
cluster_name="$2"

if [[ -z "${project}" || -z "${cluster_name}" ]]; then
	echo "Usage: $0 <project> <cluster_name>"
	exit 1
fi

if ! type -P gcloud 1>/dev/null; then
	echo "gcloud is not available and your compute resources are not being cleaned up"
	echo "https://console.cloud.google.com/compute/instances?project=${project}"
	exit 1
fi

echo "Deleting compute nodes"
node_filter="labels.slurm_cluster_name=${cluster_name} AND labels.slurm_instance_role=compute"
while true; do
	nodes=$(gcloud compute instances list --project "${project}" --format="value(selfLink)" --filter="${node_filter}" --limit=10 | paste -sd " " -)
	if [[ -z "${nodes}" ]]; then
		break
	fi
	# The lack of quotes is intentional and causes each new space-separated "word" to
	# be treated as independent arguments. See PR#2523
	# shellcheck disable=SC2086
	gcloud compute instances delete --quiet ${nodes}
done

echo "Deleting resource policies"
policies_filter="name:${cluster_name}-*"
while true; do
	policies=$(gcloud compute resource-policies list --project "${project}" --format="value(selfLink)" --filter="${policies_filter}" --limit=10 | paste -sd " " -)
	if [[ -z "${policies}" ]]; then
		break
	fi
	# The lack of quotes is intentional and causes each new space-separated "word" to
	# be treated as independent arguments. See PR#2523
	# shellcheck disable=SC2086
	gcloud compute resource-policies delete --quiet ${policies}
done
