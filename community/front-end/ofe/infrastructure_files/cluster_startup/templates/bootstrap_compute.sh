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
SPACK_DIR={{ spack_dir }}
USE_CONTAINERS={{ use_containers }}

echo "This is the startup script for the compute nodes on cluster ${CLUSTER_ID}"

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
	# Skip unreachable repos (e.g. an image's CUDA repo) instead of failing every
	# dnf transaction: set it in [main] (Rocky ships False) and in each repo.
	dnf_conf=/etc/dnf/dnf.conf
	[[ -f ${dnf_conf} ]] || dnf_conf=/etc/yum.conf
	if grep -q '^[[:space:]]*skip_if_unavailable[[:space:]]*=' "${dnf_conf}"; then
		sed -i --follow-symlinks 's/^[[:space:]]*skip_if_unavailable[[:space:]]*=.*/skip_if_unavailable=True/' "${dnf_conf}"
	else
		sed -i --follow-symlinks '/^\[main\]/a skip_if_unavailable=True' "${dnf_conf}"
	fi
	sed -i 's/^[[:space:]]*skip_if_unavailable[[:space:]]*=.*/skip_if_unavailable=True/' /etc/yum.repos.d/*.repo 2>/dev/null || true
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
spack_dir=${SPACK_DIR}
use_containers=${USE_CONTAINERS}
EOF

exec ansible-playbook ./compute.yaml
