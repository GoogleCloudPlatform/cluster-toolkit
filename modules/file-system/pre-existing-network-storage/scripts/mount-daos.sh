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

OS_ID=$(awk -F '=' '/^ID=/ {print $2}' /etc/os-release | sed -e 's/"//g')
OS_VERSION=$(awk -F '=' '/VERSION_ID/ {print $2}' /etc/os-release | sed -e 's/"//g')
OS_VERSION_MAJOR=$(awk -F '=' '/VERSION_ID/ {print $2}' /etc/os-release | sed -e 's/"//g' -e 's/\..*$//')

# Parse local_mount, mount_options from argument.
# Format mount-options string to be compatible to dfuse mount command.
# e.g. "disable-wb-cache,eq-count=8" --> --disable-wb-cache --eq-count=8.
for arg in "$@"; do
	if [[ $arg == --access_points=* ]]; then
		access_points="${arg#*=}"
	fi
	if [[ $arg == --local_mount=* ]]; then
		local_mount="${arg#*=}"
	fi
	if [[ $arg == --mount_options=* ]]; then
		mount_options="${arg#*=}"
		mount_options="--${mount_options//,/ --}"
	fi
done

# Edit agent config
daos_config=/etc/daos/daos_agent.yml
sed -i "s/#.*transport_config/transport_config/g" $daos_config
sed -i "s/#.*allow_insecure:.*false/  allow_insecure: true/g" $daos_config
sed -i "s/.*access_points.*/access_points: $access_points/g" $daos_config

# Get interface names with "s0f0" suffix
if ifconfig -a | grep 's0f0'; then
	sof0_interfaces=$(ifconfig -a | grep 's0f0:' | awk '{print $1}' | tr ':' '\n' | grep -v '^$' | awk '!a[$0]++' | sed 's/^/"/g' | sed 's/$/"/g' | paste -sd, -)

	# Append the sof0_interfaces to the existing list
	exclude_fabric_ifaces="lo,$sof0_interfaces"

	# Update the file with the new list
	sed -i "s/#.*exclude_fabric_ifaces: \[.*/exclude_fabric_ifaces: [$exclude_fabric_ifaces]/" $daos_config
fi

# Start service
if { [ "${OS_ID}" = "rocky" ] || [ "${OS_ID}" = "rhel" ]; } && { [ "${OS_VERSION_MAJOR}" = "8" ] || [ "${OS_VERSION_MAJOR}" = "9" ]; }; then
	# TODO: Update script to change default log destination folder, after daos_agent user is supported in debian and ubuntu.
	# Move agent log destination from /tmp/ (default) to /var/log/daos_agent/
	mkdir -p /var/log/daos_agent
	chown daos_agent:daos_agent /var/log/daos_agent
	sed -i "s/#.*log_file:.*/log_file: \/var\/log\/daos_agent\/daos_agent.log/g" $daos_config
	systemctl enable daos_agent.service
	systemctl start daos_agent.service
elif { [ "${OS_ID}" = "ubuntu" ] && [ "${OS_VERSION}" = "22.04" ]; } || { [ "${OS_ID}" = "debian" ] && [ "${OS_VERSION_MAJOR}" = "12" ]; }; then
	mkdir -p /var/run/daos_agent
	daos_agent -o /etc/daos/daos_agent.yml >/dev/null 2>&1 &
else
	echo "Unsupported operating system ${OS_ID} ${OS_VERSION}. This script only supports Rocky Linux 8, Redhat 8, Redhat 9, Ubuntu 22.04, and Debian 12."
	exit 1
fi

# Mount parallelstore instance to client vm.
mkdir -p "$local_mount"
chmod 777 "$local_mount"

# Mount container for multi-user.
fuse_config=/etc/fuse.conf
sed -i "s/#.*user_allow_other/user_allow_other/g" $fuse_config

# make sure limit of open files is high enough for dfuse (1M of open files)
ulimit -n 1048576

# Store the mounting logic in a variable
mount_command="if mountpoint -q '$local_mount'; then fusermount3 -u '$local_mount'; fi; for i in {1..10}; do /bin/dfuse -m '$local_mount' --pool default-pool --container default-container --multi-user $mount_options --foreground && break; sleep 1; done"

# Construct the service name with the local_mount suffix
service_name="mount_parallelstore_${local_mount//\//_}.service"

# --- Begin: Add systemd service creation ---
cat >/usr/lib/systemd/system/"${service_name}" <<EOF
[Unit]
Description=DAOS Mount Service
After=network-online.target daos_agent.service

[Service]
Type=simple
User=root
Group=root
Restart=always
RestartSec=1
ExecStart=/bin/bash -c "$mount_command"
ExecStop=fusermount3 -u '$local_mount'

[Install]
WantedBy=multi-user.target
EOF

systemctl enable "${service_name}"
systemctl start "${service_name}"
# --- End: Add systemd service creation ---

exit 0
