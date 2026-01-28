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

echo "This is the startup script for the login nodes on cluster ${CLUSTER_ID}"

# Install ansible
# Download ansible playbook from GCS bucket
# Set up our facts file
# Run ansible for controller

set -x
set -e
if [[ $(type -P yum) ]]; then
	yum install -y ansible
else
	apt install -y ansible
fi

cd /tmp
gsutil -m cp -r "gs://${BUCKET}/clusters/ansible_setup" /tmp
cd /tmp/ansible_setup

# Set up facts file
mkdir -p /etc/ansible/facts.d
cat >/etc/ansible/facts.d/ghpcfe.fact <<EOF
[config]
cluster_id=${CLUSTER_ID}
cluster_bucket=${BUCKET}
spack_dir=${SPACK_DIR}
EOF

exec ansible-playbook ./login.yaml
