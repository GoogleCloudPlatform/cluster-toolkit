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
nodeset_name="$3"
universe_domain="$4"
compute_endpoint_version="$5"
gcloud_dir="$6"

if [[ $# -ne 5 ]] && [[ $# -ne 6 ]]; then
	echo "Usage: $0 <project> <cluster_name> <nodeset_name> <universe_domain> <compute_endpoint_version> [<gcloud_dir>]"
	exit 1
fi

if [[ -n "${gcloud_dir}" ]]; then
	export PATH="$gcloud_dir:$PATH"
fi

export CLOUDSDK_API_ENDPOINT_OVERRIDES_COMPUTE="https://www.${universe_domain}/compute/${compute_endpoint_version}/"
export CLOUDSDK_CORE_PROJECT="${project}"

if ! type -P gcloud 1>/dev/null; then
	echo "gcloud is not available and your compute resources are not being cleaned up"
	echo "https://console.cloud.google.com/compute/instances?project=${project}"
	exit 1
fi

echo "Deleting compute nodes"
node_filter="name:${cluster_name}-${nodeset_name}-* labels.slurm_cluster_name=${cluster_name} AND labels.slurm_instance_role=compute"

tmpfile=$(mktemp) # have to use a temp file, since `< <(gcloud ...)` doesn't work nicely with `head`
trap 'rm -f "$tmpfile"' EXIT

running_nodes_filter="${node_filter} AND status!=STOPPING"
# List all currently running instances and attempt to delete them
gcloud compute instances list --format="value(selfLink)" --filter="${running_nodes_filter}" >"$tmpfile"
# Do 10 instances at a time
while batch="$(head -n 10)" && [[ ${#batch} -gt 0 ]]; do
	nodes=$(echo "$batch" | paste -sd " " -) # concat into a single space-separated line
	# The lack of quotes around ${nodes} is intentional and causes each new space-separated "word" to
	# be treated as independent arguments. See PR#2523
	# shellcheck disable=SC2086
	gcloud compute instances delete --quiet ${nodes} || echo "Failed to delete some instances"
done <"$tmpfile"

# In case if controller tries to delete the nodes as well,
# wait until nodes in STOPPING state are deleted, before deleting the resource policies
stopping_nodes_filter="${node_filter} AND status=STOPPING"
while true; do
	node=$(gcloud compute instances list --format="value(name)" --filter="${stopping_nodes_filter}" --limit=1)
	if [[ -z "${node}" ]]; then
		break
	fi
	echo "Waiting for instances to be deleted: ${node}"
	sleep 5
done

echo "Deleting resource policies"
policies_filter="name:${cluster_name}-${nodeset_name}-slurmgcp-managed-*"
gcloud compute resource-policies list --format="value(selfLink)" --filter="${policies_filter}" | while read -r line; do
	echo "Deleting resource policy: $line"
	gcloud compute resource-policies delete --quiet "${line}" || {
		echo "Failed to delete resource policy: $line"
	}
done
