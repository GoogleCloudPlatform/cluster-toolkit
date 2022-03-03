#!/bin/bash

BUCKET={{ server_bucket }}
CLUSTER_ID={{ cluster.id }}

echo "This is the startup script for the controller on cluster ${CLUSTER_ID}"

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
mkdir facts.d
cat > facts.d/ghpcfe.fact <<EOF
[config]
cluster_id=${CLUSTER_ID}
cluster_bucket=${BUCKET}
spack_dir={{ spack_dir }}
fec2_subscription={{ fec2_subscription }}
fec2_topic={{ fec2_topic }}
EOF

exec ansible-playbook ./controller.yaml
