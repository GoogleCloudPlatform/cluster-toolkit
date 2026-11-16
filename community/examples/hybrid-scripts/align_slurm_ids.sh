#!/bin/bash
#
# Copyright 2026 Google LLC
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

# Align the cloud "slurm" user/group uid/gid with the on-prem values so a
# hybrid compute node matches the controller. Meant to be baked into the
# compute-node image (run once at image build time), not at every node boot.
#
# If the target uid/gid is already held by another account, that account is
# first relocated to a free id and its files are re-owned, freeing the target
# for slurm.
#
# Usage: align_slurm_ids.sh <TARGET_UID> <TARGET_GID> [SLURM_USER] [SLURM_GROUP]

set -e -o pipefail

TARGET_UID="$1"
TARGET_GID="$2"
SLURM_USER="${3:-slurm}"
SLURM_GROUP="${4:-slurm}"

if [ -z "$TARGET_UID" ] || [ -z "$TARGET_GID" ]; then
	echo "usage: $0 <TARGET_UID> <TARGET_GID> [SLURM_USER] [SLURM_GROUP]" >&2
	exit 1
fi

# Range used to relocate an account that already occupies the target id. Kept
# high to stay clear of system accounts and the nobody/nogroup (65534) and -1
# sentinel (65535) ids.
RELOCATE_START=60000
RELOCATE_END=64999

find_free_id() {
	# $1: "passwd" or "group"
	local db="$1" id
	for ((id = RELOCATE_START; id <= RELOCATE_END; id++)); do
		if ! getent "$db" "$id" >/dev/null; then
			echo "$id"
			return 0
		fi
	done
	echo "no free id available in ${RELOCATE_START}-${RELOCATE_END}" >&2
	return 1
}

free_target_gid() {
	local gid="$1" occupant new_gid
	occupant="$(getent group "$gid" | cut -d: -f1)" || true
	[ -z "$occupant" ] && return 0               # gid is free
	[ "$occupant" = "$SLURM_GROUP" ] && return 0 # already ours
	new_gid="$(find_free_id group)"
	echo "gid ${gid} held by group '${occupant}'; relocating it to ${new_gid}"
	groupmod -g "$new_gid" "$occupant"
	# groupmod does not re-group existing files; fix them on the local root fs.
	# -xdev keeps us off network mounts (e.g. NFS).
	find / -xdev -gid "$gid" -exec chgrp -h "$new_gid" {} + || true
}

free_target_uid() {
	local uid="$1" occupant new_uid
	occupant="$(getent passwd "$uid" | cut -d: -f1)" || true
	[ -z "$occupant" ] && return 0              # uid is free
	[ "$occupant" = "$SLURM_USER" ] && return 0 # already ours
	new_uid="$(find_free_id passwd)"
	echo "uid ${uid} held by user '${occupant}'; relocating it to ${new_uid}"
	usermod -u "$new_uid" "$occupant"
	# usermod only re-owns files under the user's home; fix the rest on the
	# local root fs. -xdev keeps us off network mounts (e.g. NFS).
	find / -xdev -uid "$uid" -exec chown -h "$new_uid" {} + || true
}

CUR_GID="$(getent group "$SLURM_GROUP" | cut -d: -f3)"
if [ "$CUR_GID" != "$TARGET_GID" ]; then
	free_target_gid "$TARGET_GID"
	groupmod -g "$TARGET_GID" "$SLURM_GROUP"
fi

CUR_UID="$(id -u "$SLURM_USER")"
if [ "$CUR_UID" != "$TARGET_UID" ]; then
	free_target_uid "$TARGET_UID"
	usermod -u "$TARGET_UID" "$SLURM_USER"
fi

# Re-own slurm's pre-existing files that still carry the old ids. usermod
# re-owns the home dir automatically; this covers everything else on the local
# root fs. -xdev keeps us off network mounts (e.g. NFS).
if [ "$CUR_UID" != "$TARGET_UID" ]; then
	find / -xdev -uid "$CUR_UID" -exec chown -h "$TARGET_UID" {} + || true
fi
if [ "$CUR_GID" != "$TARGET_GID" ]; then
	find / -xdev -gid "$CUR_GID" -exec chgrp -h "$TARGET_GID" {} + || true
fi

echo "slurm now uid=$(id -u "$SLURM_USER") gid=$(getent group "$SLURM_GROUP" | cut -d: -f3)"
