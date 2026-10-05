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

# shellcheck disable=SC1083
BUCKET={{ server_bucket }}
CLUSTER_ID={{ cluster.id }}
USE_CONTAINERS={{ use_containers }}

echo "This is the startup script for the controller on cluster ${CLUSTER_ID}"

# Install ansible
# Download ansible playbook from GCS bucket
# Set up our facts file
# Run ansible for controller

set -x
set -e
if [[ $(type -P yum) ]]; then
	# dnf-automatic upgrades packages on first boot, and a yum left waiting on its
	# lock then fails with DB_VERSION_MISMATCH. Stop the timer; let any run finish.
	systemctl stop dnf-automatic.timer 2>/dev/null || true
	while [[ $(systemctl is-active dnf-automatic.service 2>/dev/null) =~ ^(activating|active|deactivating)$ ]]; do
		sleep 5
	done
	# A single unreachable repo preconfigured in the image (e.g. a CUDA repo) would
	# otherwise fail every dnf transaction, even for unrelated packages. Let dnf skip
	# any repo whose metadata cannot be fetched instead of aborting the whole node.
	for repo_file in /etc/yum.repos.d/*.repo; do
		grep -q '^skip_if_unavailable=1$' "${repo_file}" 2>/dev/null ||
			sed -i '/^\[.*\]$/a skip_if_unavailable=1' "${repo_file}" 2>/dev/null || true
	done
	yum install -y ansible
else
	apt install -y ansible
fi

cd /tmp
gcloud storage cp --recursive "gs://${BUCKET}/clusters/ansible_setup" /tmp
cd /tmp/ansible_setup

# Set up facts file
mkdir -p /etc/ansible/facts.d
cat >/etc/ansible/facts.d/ghpcfe.fact <<EOF
[config]
cluster_id=${CLUSTER_ID}
cluster_bucket=${BUCKET}
spack_dir={{ spack_dir }}
fec2_subscription={{ fec2_subscription }}
fec2_topic={{ fec2_topic }}
use_containers=${USE_CONTAINERS}
EOF

exec ansible-playbook ./controller.yaml
