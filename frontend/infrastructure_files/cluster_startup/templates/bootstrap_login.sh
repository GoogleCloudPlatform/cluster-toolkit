#!/bin/bash

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
yum install -y ansible

cd /tmp
gsutil -m cp -r gs://${BUCKET}/clusters/ansible_setup /tmp
cd /tmp/ansible_setup

# Set up facts file
mkdir -p /etc/ansible/facts.d
cat > /etc/ansible/facts.d/ghpcfe.fact <<EOF
[config]
cluster_id=${CLUSTER_ID}
cluster_bucket=${BUCKET}
spack_dir=${SPACK_DIR}
EOF

exec ansible-playbook ./login.yaml
