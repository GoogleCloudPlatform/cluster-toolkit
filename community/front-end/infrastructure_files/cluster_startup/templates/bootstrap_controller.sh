#!/bin/bash

# shellcheck disable=SC1083
BUCKET={{ server_bucket }}
CLUSTER_ID={{ cluster.id }}

echo "This is the startup script for the controller on cluster ${CLUSTER_ID}"

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
spack_dir={{ spack_dir }}
fec2_subscription={{ fec2_subscription }}
fec2_topic={{ fec2_topic }}
EOF

exec ansible-playbook ./controller.yaml
