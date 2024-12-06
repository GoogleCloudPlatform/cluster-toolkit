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
OS_ID_LIKE=$(awk -F '=' '/^ID_LIKE=/ {print $2}' /etc/os-release | sed -e 's/"//g')
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

# Get names of network interfaces not in first PCI slot
# The first PCI slot is a standard network adapter while remaining interfaces
# are typically network cards dedicated to GPU or workload communication
if [[ "$OS_ID_LIKE" == "debian" ]]; then
	extra_interfaces=$(find /sys/class/net/ -not -name 'enp0s*' -regextype posix-extended -regex '.*/enp[0-9]+s.*' -printf '"%f"\n' | paste -s -d ',')
elif [[ "$OS_ID_LIKE" =~ "rhel" ]]; then
	extra_interfaces=$(find /sys/class/net/ -not -name eth0 -regextype posix-extended -regex '.*/eth[0-9]+' -printf '"%f"\n' | paste -s -d ',')
fi

if [[ -n "$extra_interfaces" ]]; then
	exclude_fabric_ifaces="\"lo\",$extra_interfaces"
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

# Construct the service name with the local_mount suffix
safe_mount_name=$(systemd-escape -p "${local_mount}")
service_name="mount_parallelstore_${safe_mount_name}.service"

# --- Begin: Add systemd service creation ---
cat >/etc/systemd/system/"${service_name}" <<EOF
[Unit]
Description=DAOS Mount Service
After=network-online.target daos_agent.service
Before=slurmd.service
ConditionPathIsMountPoint=!${local_mount}

[Service]
Type=simple
User=root
Group=root
Restart=on-failure
RestartSec=10
ExecStart=/bin/dfuse -m $local_mount --pool default-pool --container default-container --multi-user $mount_options --foreground
ExecStop=/usr/bin/fusermount3 -u $local_mount

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "${service_name}"
systemctl start "${service_name}"
# --- End: Add systemd service creation ---

exit 0
