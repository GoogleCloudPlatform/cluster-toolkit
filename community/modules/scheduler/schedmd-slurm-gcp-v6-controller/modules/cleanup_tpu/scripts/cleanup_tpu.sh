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
zone="$4"
universe_domain="$5"
compute_endpoint_version="$6"
gcloud_dir="$7"

if [[ $# -ne 6 ]] && [[ $# -ne 7 ]]; then
	echo "Usage: $0 <project> <cluster_name> <nodeset_name> <zone> <universe_domain> <compute_endpoint_version> [<gcloud_dir>]"
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

echo "Deleting TPU nodes"
node_filter="name~${cluster_name}-${nodeset_name}"
running_nodes_filter="${node_filter} AND state!=DELETING"

# List all currently running nodes and attempt to delete them
gcloud compute tpus tpu-vm list --zone="${zone}" --format="value(name)" --filter="${running_nodes_filter}" | while read -r name; do
	echo "Deleting TPU node: $name"
	gcloud compute tpus tpu-vm delete --async --zone="${zone}" --quiet "${name}" || echo "Failed to delete $name"
done

# Wait until nodes in DELETING state are deleted, before deleting the resource policies
deleting_nodes_filter="${node_filter} AND state=DELETING"
while true; do
	node=$(gcloud compute tpus tpu-vm list --zone="${zone}" --format="value(name)" --filter="${deleting_nodes_filter}" --limit=1)
	if [[ -z "${node}" ]]; then
		break
	fi
	echo "Waiting for nodes to be deleted: ${node}"
	sleep 5
done
