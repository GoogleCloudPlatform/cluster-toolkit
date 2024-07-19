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

set -e

# Parse local_mount, mount_options from argument.
# Format mount-options string to be compatible to dfuse mount command.
# e.g. "disable-wb-cache,eq-count=8" --> --disable-wb-cache --eq-count=8.
for arg in "$@"; do
	if [[ $arg == --local_mount=* ]]; then
		local_mount="${arg#*=}"
	fi
	if [[ $arg == --mount_options=* ]]; then
		mount_options="${arg#*=}"
		mount_options="--${mount_options//,/ --}"
	fi
done

# Mount parallelstore instance to client vm.
mkdir -p "$local_mount"
chmod 777 "$local_mount"

# Mount container for multi-user.
fuse_config=/etc/fuse.conf
sed -i "s/#.*user_allow_other/user_allow_other/g" $fuse_config
# To parse mount_options as --disable-wb-cache --eq-count=8.
# shellcheck disable=SC2086
dfuse -m "$local_mount" --pool default-pool --container default-container --multi-user $mount_options

exit 0
