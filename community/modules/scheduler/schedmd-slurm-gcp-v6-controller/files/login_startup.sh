#!/bin/bash

set -ex

SLURM_DIR=/slurm
FLAGFILE=$SLURM_DIR/slurm_configured_do_not_remove
SCRIPTS_DIR=$SLURM_DIR/scripts
if [[ -z "$HOME" ]]; then
    # google-startup-scripts.service lacks environment variables
    HOME="$(getent passwd "$(whoami)" | cut -d: -f6)"
fi

for i in $(seq 10); do
    [[ $i -gt 1 ]] && sleep 5;
    ping -q -w1 -c1 metadata.google.internal > /dev/null && s=0 && break || s=$?;
    echo "ERROR: Failed to contact metadata server, will retry"
done
if [[ $s -ne 0 ]]; then
    echo "ERROR: Unable to contact metadata server, aborting"
    wall -n '*** Slurm setup failed in the startup script! see `journalctl -u google-startup-scripts` ***'
    exit 1
else
    echo "INFO: Successfully contacted metadata server"
fi

for i in $(seq 5); do
    [[ $i -gt 1 ]] && sleep 2;
    ping -q -w1 -c1 8.8.8.8 > /dev/null && s=0 && break || s=$?;
    echo "failed to ping Google DNS, will retry"
done
if [[ $s -ne 0 ]]; then
    echo "WARNING: No internet access detected"
else
    echo "INFO: Internet access detected"
fi

mkdir -p $SCRIPTS_DIR
URL="http://metadata.google.internal/computeMetadata/v1"
CLUSTER_NAME=$(curl -sS --fail --header Metadata-Flavor:Google $URL/instance/attributes/slurm_cluster_name)
MOUNT_BUCKET=$(curl -sS --fail --header Metadata-Flavor:Google $URL/instance/attributes/slurm_bucket_mount)

if [[ $MOUNT_BUCKET != "true" ]]; then
    echo "ERROR: slurm_bucket_mount should be set to `true`"
    exit 1
fi

echo "$CLUSTER_NAME-controller:/slurm/bucket /slurm/bucket     nfs     defaults,hard,intr,_netdev     0 0" >> /etc/fstab
systemctl daemon-reload
mkdir -p /slurm/bucket

until mount /slurm/bucket; do
  echo "WARN: Could not mount config bucket, retrying in 5 seconds."
  sleep 5
done

if [[ ! -f /slurm/bucket/slurm-gcp-devel.zip ]]; then
    echo "ERROR: Could not download SlurmGCP scripts"
    exit 1
fi

unzip -o /slurm/bucket/slurm-gcp-devel.zip -d "$SCRIPTS_DIR"

#temporary hack to not make the script fail on TPU vm
chown slurm:slurm -R "$SCRIPTS_DIR" || true
chmod 700 -R "$SCRIPTS_DIR"

if [[ -f $FLAGFILE ]]; then
    echo "WARNING: Slurm was previously configured, quitting"
    exit 0
fi
touch $FLAGFILE

echo "INFO: Running python cluster setup script"
SETUP_SCRIPT_FILE=$SCRIPTS_DIR/setup.py
chmod +x $SETUP_SCRIPT_FILE
exec $SETUP_SCRIPT_FILE