#!/bin/bash
# Copyright 2025 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e
set -o xtrace

SLURM_DIR=/slurm
FLAGFILE=$SLURM_DIR/slurm_configured_do_not_remove
SCRIPTS_DIR=$SLURM_DIR/scripts
if [[ -z "$HOME" ]]; then
	# google-startup-scripts.service lacks environment variables
	HOME="$(getent passwd "$(whoami)" | cut -d: -f6)"
fi

METADATA_SERVER="metadata.google.internal"
URL="http://$METADATA_SERVER/computeMetadata/v1"
CURL="curl -sS --fail --header Metadata-Flavor:Google"

PING_METADATA="ping -q -w1 -c1 $METADATA_SERVER"
echo "INFO: $PING_METADATA"
for i in $(seq 10); do
	[ "$i" -gt 1 ] && sleep 5
	$PING_METADATA >/dev/null && s=0 && break || s=$?
	echo "ERROR: Failed to contact metadata server, will retry"
done
if [ "$s" -ne 0 ]; then
	echo "ERROR: Unable to contact metadata server, aborting"
	# shellcheck disable=SC2016
	wall -n '*** Slurm setup failed in the startup script! see `journalctl -u google-startup-scripts` ***'
	exit 1
else
	echo "INFO: Successfully contacted metadata server"
fi

mkdir -p $SCRIPTS_DIR
UNIVERSE_DOMAIN="$($CURL $URL/instance/attributes/universe_domain)"
BUCKET="$($CURL $URL/instance/attributes/slurm_bucket_path)"
if [[ -z $BUCKET ]]; then
	echo "ERROR: No bucket path detected."
	exit 1
fi

SCRIPTS_ZIP="$HOME/slurm-gcp-scripts.zip"
export CLOUDSDK_CORE_UNIVERSE_DOMAIN="$UNIVERSE_DOMAIN"
until gcloud storage cp "$BUCKET/slurm-gcp-devel.zip" "$SCRIPTS_ZIP"; do
	echo "WARN: Could not download SlurmGCP scripts, retrying in 5 seconds."
	# Remove marker used to determine if gcloud is being used in a GCE VM.
	# This can get mistakenly set to False in some cases.
	rm -f /root/.config/gcloud/gce
	sleep 5
done
unzip -o "$SCRIPTS_ZIP" -d "$SCRIPTS_DIR"
rm -rf "$SCRIPTS_ZIP"

#temporary hack to not make the script fail on TPU vm
chown slurm:slurm -R "$SCRIPTS_DIR" || true
chmod 700 -R "$SCRIPTS_DIR"

if [ -f $FLAGFILE ]; then
	echo "WARNING: Slurm was previously configured, quitting"
	exit 0
fi
touch $FLAGFILE

function tpu_setup {
	#allow the following command to fail, as this attribute does not exist for regular nodes
	docker_image=$($CURL $URL/instance/attributes/slurm_docker_image 2>/dev/null || true)
	if [ -z "$docker_image" ]; then #Not a tpu node, do not do anything
		return
	fi
	if [ "$OS_ENV" == "slurm_container" ]; then #Already inside the slurm container, we should continue starting
		return
	fi

	# enable transparent hugepages
	# TPU runtime startup and shutdown time should be significantly improved on TPU v5e and newer.

	echo always >/sys/kernel/mm/transparent_hugepage/enabled

	# remove everything after a slash from docker_image
	docker_image_host=${docker_image/\/*/}
	#Prepare to docker pull with gcloud
	mkdir -p /root/.docker
	cat <<EOF >/root/.docker/config.json
{
  "credHelpers": {
    "gcr.io": "gcloud",
    "${docker_image_host}": "gcloud"
  }
}
EOF
	#cgroup detection
	CGV=1
	CGROUP_FLAGS="-v /sys/fs/cgroup:/sys/fs/cgroup:rw"
	if [ -f /sys/fs/cgroup/cgroup.controllers ]; then #CGV2
		CGV=2
	fi
	if [ $CGV == 2 ]; then
		CGROUP_FLAGS="--cgroup-parent=docker.slice --cgroupns=private --tmpfs /run --tmpfs /run/lock --tmpfs /tmp:exec,nosuid,mode=1777"
		if [ ! -f /etc/systemd/system/docker.slice ]; then #In case that there is no slice prepared for hosting the containers create it
			printf "[Unit]\nDescription=docker slice\nBefore=slices.target\n[Slice]\nCPUAccounting=true\nMemoryAccounting=true" >/etc/systemd/system/docker.slice
			systemctl start docker.slice
		fi
	fi
	#for the moment always use --privileged, as systemd might not work properly otherwise
	TPU_FLAGS="--privileged"
	# TPU_FLAGS="--cap-add SYS_RESOURCE --device /dev/accel0 --device /dev/accel1 --device /dev/accel2 --device /dev/accel3"
	# if [ $CGV == 2 ]; then #In case that we are in CGV2 for systemd to work correctly for the moment we go with privileged
	# 	TPU_FLAGS="--privileged"
	# fi

	# pass the hostname into slurmd container, so the work distribution frameworks will be able to correctly
	# identify local address and hostname and coordinate the work
	# shellcheck disable=SC2086
	docker run --env OS_ENV=slurm_container -d $CGROUP_FLAGS $TPU_FLAGS --net=host --name=slurmd --hostname="$(hostname -s)" --domainname="$(hostname -d)" --entrypoint=/usr/bin/systemd --restart unless-stopped "$docker_image"
	exit 0
}

tpu_setup #will do nothing for normal nodes or the container spawned inside TPU

echo "INFO: Running python cluster setup script"
SETUP_SCRIPT_FILE=$SCRIPTS_DIR/setup.py
chmod +x $SETUP_SCRIPT_FILE
exec $SETUP_SCRIPT_FILE
