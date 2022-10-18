#!/bin/bash
# Copyright 2022 Google LLC
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

SERVER_IP=$1
REMOTE_MOUNT_WITH_SLASH=$2
LOCAL_MOUNT=$3
FS_TYPE=$4
MOUNT_OPTIONS=$5

MOUNT_IDENTIFIER="${SERVER_IP}:${REMOTE_MOUNT_WITH_SLASH}[[:space:]]${LOCAL_MOUNT}"
if ! grep -q "${MOUNT_IDENTIFIER}" /etc/fstab; then
	echo "Adding ${SERVER_IP}:${REMOTE_MOUNT_WITH_SLASH} -> ${LOCAL_MOUNT} to /etc/fstab"

	[[ -z "${MOUNT_OPTIONS}" ]] && POPULATED_MOUNT_OPTIONS="defaults" || POPULATED_MOUNT_OPTIONS="${MOUNT_OPTIONS}"
	echo "${SERVER_IP}:${REMOTE_MOUNT_WITH_SLASH} ${LOCAL_MOUNT} ${FS_TYPE} ${POPULATED_MOUNT_OPTIONS} 0 0" >>/etc/fstab
else
	echo "Skipping editing /etc/fstab as entry already exists"
fi

if ! findmnt --source "${SERVER_IP}":"${REMOTE_MOUNT_WITH_SLASH}" --target "${LOCAL_MOUNT}" &>/dev/null; then
	echo "Mounting --target ${LOCAL_MOUNT} from fstab"
	mkdir -p "${LOCAL_MOUNT}"
	mount --target "${LOCAL_MOUNT}"
else
	echo "Skipping mounting source: ${SERVER_IP}:${REMOTE_MOUNT_WITH_SLASH}, already mounted to target:${LOCAL_MOUNT}"
fi
