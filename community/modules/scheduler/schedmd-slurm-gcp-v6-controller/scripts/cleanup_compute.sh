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
universe_domain="$3"
compute_endpoint_version="$4"
gcloud_dir="$5"

if [[ -z "${project}" || -z "${cluster_name}" || -z "${universe_domain}" || -z "${compute_endpoint_version}" ]]; then
	echo "Usage: $0 <project> <cluster_name> <universe_domain> <compute_endpoint_version> <gcloud_dir>"
	exit 1
fi

if [[ -n "${gcloud_dir}" ]]; then
	export PATH="$gcloud_dir:$PATH"
fi

API_ENDPOINT="CLOUDSDK_API_ENDPOINT_OVERRIDES_COMPUTE=https://www.${universe_domain}/compute/${compute_endpoint_version}/"

if ! type -P gcloud 1>/dev/null; then
	echo "gcloud is not available and your compute resources are not being cleaned up"
	echo "https://console.cloud.google.com/compute/instances?project=${project}"
	exit 1
fi

compute_cmd="$API_ENDPOINT gcloud compute"

echo "Deleting compute nodes"
base_node_filter="labels.slurm_cluster_name=${cluster_name} AND labels.slurm_instance_role=compute"
running_node_filter="${base_node_filter} AND status=RUNNING"
while true; do
	nodes=$(bash -c "$compute_cmd instances list --project \"${project}\" --format=\"value(selfLink)\" --filter=\"${node_filter}\" --limit=16" | paste -sd " " -)
	if [[ -z "${nodes}" ]]; then
		break
	fi
	# The lack of quotes is intentional and causes each new space-separated "word" to
	# be treated as independent arguments. See PR#2523
	# shellcheck disable=SC2086
	bash -c "$compute_cmd instances delete --quiet ${nodes}" || echo "delete failed, retrying"
done

while true; do
	nodes=$(bash -c "$compute_cmd instances list --project \"${project}\" --format=\"value(name)\" --filter=\"${node_filter}\" --limit=1" | paste -sd " " -)
	if [[ -z "${nodes}" ]]; then
		break
	fi
	echo "Waiting for compute nodes to be deleted, ${nodes} still being deleted"
	sleep 5
done

echo "Deleting resource policies"
policies_filter="name:${cluster_name}-*"
while true; do
	policies=$(bash -c "$compute_cmd resource-policies list --project \"${project}\" --format=\"value(selfLink)\" --filter=\"${policies_filter}\" --limit=16" | paste -sd " " -)
	if [[ -z "${policies}" ]]; then
		break
	fi
	# The lack of quotes is intentional and causes each new space-separated "word" to
	# be treated as independent arguments. See PR#2523
	# shellcheck disable=SC2086
	bash -c "$compute_cmd resource-policies delete --quiet ${policies}" || echo "delete failed, retrying"
done
