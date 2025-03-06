#!/bin/bash
# Copyright (C) SchedMD LLC.
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
    [ $i -gt 1 ] && sleep 5;
    $PING_METADATA > /dev/null && s=0 && break || s=$?;
    echo "ERROR: Failed to contact metadata server, will retry"
done
if [ $s -ne 0 ]; then
    echo "ERROR: Unable to contact metadata server, aborting"
    wall -n '*** Slurm setup failed in the startup script! see `journalctl -u google-startup-scripts` ***'
    exit 1
else
    echo "INFO: Successfully contacted metadata server"
fi

PING_GOOGLE="ping -q -w1 -c1 8.8.8.8"
echo "INFO: $PING_GOOGLE"
for i in $(seq 5); do
    [ $i -gt 1 ] && sleep 2;
    $PING_GOOGLE > /dev/null && s=0 && break || s=$?;
	echo "failed to ping Google DNS, will retry"
done
if [ $s -ne 0 ]; then
    echo "WARNING: No internet access detected"
else
    echo "INFO: Internet access detected"
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
	docker_image=$($CURL $URL/instance/attributes/slurm_docker_image 2> /dev/null || true)
	if [ -z $docker_image ]; then #Not a tpu node, do not do anything
		return
	fi
	if [ "$OS_ENV" == "slurm_container" ]; then #Already inside the slurm container, we should continue starting
		return
	fi

	#given a input_string like "WORKER_0:Joseph;WORKER_1:richard;WORKER_2:edward;WORKER_3:john" and a number 1, this function will print richard
	parse_metadata() {
		local number=$1
		local input_string=$2
		local word=$(echo "$input_string" | awk -v n="$number" -F ':|;' '{ for (i = 1; i <= NF; i+=2) if ($(i) == "WORKER_"n) print $(i+1) }')
		echo "$word"
	}

	input_string=$($CURL $URL/instance/attributes/slurm_names)
	worker_id=$($CURL $URL/instance/attributes/tpu-env | awk '/WORKER_ID/ {print $2}' | tr -d \')
	real_name=$(parse_metadata $worker_id $input_string)

	#Prepare to docker pull with gcloud
	mkdir -p /root/.docker
	cat << EOF > /root/.docker/config.json
{
  "credHelpers": {
    "gcr.io": "gcloud",
	"us-docker.pkg.dev": "gcloud"
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
		CGROUP_FLAGS="--cgroup-parent=docker.slice --cgroupns=private --tmpfs /run --tmpfs /run/lock --tmpfs /tmp"
		if [ ! -f /etc/systemd/system/docker.slice ]; then #In case that there is no slice prepared for hosting the containers create it
			printf "[Unit]\nDescription=docker slice\nBefore=slices.target\n[Slice]\nCPUAccounting=true\nMemoryAccounting=true" > /etc/systemd/system/docker.slice
			systemctl start docker.slice
		fi
	fi
	#for the moment always use --privileged, as systemd might not work properly otherwise
	TPU_FLAGS="--privileged"
	# TPU_FLAGS="--cap-add SYS_RESOURCE --device /dev/accel0 --device /dev/accel1 --device /dev/accel2 --device /dev/accel3"
	# if [ $CGV == 2 ]; then #In case that we are in CGV2 for systemd to work correctly for the moment we go with privileged
	# 	TPU_FLAGS="--privileged"
	# fi

	docker run -d $CGROUP_FLAGS $TPU_FLAGS --net=host --name=slurmd --hostname=$real_name --entrypoint=/usr/bin/systemd --restart unless-stopped $docker_image
	exit 0
}

tpu_setup #will do nothing for normal nodes or the container spawned inside TPU

echo "INFO: Running python cluster setup script"
SETUP_SCRIPT_FILE=$SCRIPTS_DIR/setup.py
chmod +x $SETUP_SCRIPT_FILE
exec $SETUP_SCRIPT_FILE
