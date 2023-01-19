#!/bin/bash
# Copyright 2023 Google LLC
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
SERVER_IP=$1
REMOTE_MOUNT=$2
LOCAL_MOUNT=$3
FS_TYPE=$4
MOUNT_OPTIONS=$5

[[ -z "${MOUNT_OPTIONS}" ]] && POPULATED_MOUNT_OPTIONS="defaults" || POPULATED_MOUNT_OPTIONS="${MOUNT_OPTIONS}"

if [ "${FS_TYPE}" = "gcsfuse" ]; then
	FS_SPEC="${REMOTE_MOUNT}"
else
	FS_SPEC="${SERVER_IP}:${REMOTE_MOUNT}"
fi

SAME_LOCAL_IDENTIFIER="^[^#].*[[:space:]]${LOCAL_MOUNT}"
EXACT_MATCH_IDENTIFIER="${FS_SPEC}[[:space:]]${LOCAL_MOUNT}[[:space:]]${FS_TYPE}[[:space:]]${POPULATED_MOUNT_OPTIONS}[[:space:]]0[[:space:]]0"

grep -q "${SAME_LOCAL_IDENTIFIER}" /etc/fstab && SAME_LOCAL_IN_FSTAB=true || SAME_LOCAL_IN_FSTAB=false
grep -q "${EXACT_MATCH_IDENTIFIER}" /etc/fstab && EXACT_IN_FSTAB=true || EXACT_IN_FSTAB=false
findmnt --source "${SERVER_IP}":"${REMOTE_MOUNT}" --target "${LOCAL_MOUNT}" &>/dev/null && EXACT_MOUNTED=true || EXACT_MOUNTED=false

# Do nothing and success if exact entry is already in fstab and mounted
if [ "$EXACT_IN_FSTAB" = true ] && [ "${EXACT_MOUNTED}" = true ]; then
	echo "Skipping mounting source: ${FS_SPEC}, already mounted to target:${LOCAL_MOUNT}"
	exit 0
fi

# Fail if previous fstab entry is using same local mount
if [ "$SAME_LOCAL_IN_FSTAB" = true ] && [ "${EXACT_IN_FSTAB}" = false ]; then
	echo "Mounting failed as local mount: ${LOCAL_MOUNT} was already in use in fstab"
	exit 1
fi

# Add to fstab if entry is not already there
if [ "${EXACT_IN_FSTAB}" = false ]; then
	echo "Adding ${FS_SPEC} -> ${LOCAL_MOUNT} to /etc/fstab"
	echo "${FS_SPEC} ${LOCAL_MOUNT} ${FS_TYPE} ${POPULATED_MOUNT_OPTIONS} 0 0" >>/etc/fstab
fi

# Mount from fstab
echo "Mounting --target ${LOCAL_MOUNT} from fstab"
mkdir -p "${LOCAL_MOUNT}"
mount --target "${LOCAL_MOUNT}"
