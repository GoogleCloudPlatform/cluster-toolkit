#!/bin/sh
# Copyright 2026 Google LLC
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

TMP_GIT_DIR=$(mktemp -d)
trap 'rm -rf "${TMP_GIT_DIR}"' EXIT INT TERM HUP

# Portable string hashing (Linux / macOS / BSD)
hash_string() {
	if command -v sha256sum >/dev/null 2>&1; then
		printf '%s' "$1" | sha256sum | cut -d" " -f1
	elif command -v shasum >/dev/null 2>&1; then
		printf '%s' "$1" | shasum -a 256 | cut -d" " -f1
	elif command -v md5sum >/dev/null 2>&1; then
		printf '%s' "$1" | md5sum | cut -d" " -f1
	elif command -v md5 >/dev/null 2>&1; then
		printf '%s' "$1" | md5 -q
	else
		printf '%s' "$1" | cksum | cut -d" " -f1
	fi
}

# Fetch source from remote Git repository using sparse checkout
fetch_git_source() {
	SRC_PATH="$1"
	REPO_URL="$2"
	REPO_REF="$3"

	if [ -z "$REPO_URL" ]; then
		echo "ERROR: REPO_URL is required for Git-based synchronization." >&2
		exit 1
	fi

	REPO_HASH=$(hash_string "${REPO_URL}-${REPO_REF}")
	CLONE_SUBDIR="${TMP_GIT_DIR}/${REPO_HASH}"
	if [ ! -d "$CLONE_SUBDIR" ]; then
		echo "--> Initializing sparse repository for: ${REPO_URL} (ref: ${REPO_REF})..." >&2
		git init "$CLONE_SUBDIR" >&2
		git -C "$CLONE_SUBDIR" remote add origin "$REPO_URL" >&2
		git -C "$CLONE_SUBDIR" sparse-checkout init --cone >&2
		if ! git -C "$CLONE_SUBDIR" fetch --depth 1 --filter=blob:none origin "$REPO_REF" >/dev/null 2>&1; then
			echo "--> Direct shallow ref fetch failed; fetching commit graph for: ${REPO_REF}..." >&2
			git -C "$CLONE_SUBDIR" fetch --filter=blob:none origin >&2
		fi
	fi
	git -C "$CLONE_SUBDIR" sparse-checkout add --skip-checks "$SRC_PATH" >&2
	git -C "$CLONE_SUBDIR" checkout "$REPO_REF" >/dev/null 2>&1 || git -C "$CLONE_SUBDIR" checkout FETCH_HEAD >&2
	printf '%s' "${CLONE_SUBDIR}/${SRC_PATH}"
}

# 1. Process Directory Sync
if [ "$MODE" = "directory" ]; then
	# Gating check: DEST_URI must never target bucket root when using delete-unmatched
	case "$DEST_URI" in
	gs://*//*)
		echo "ERROR: DEST_URI '${DEST_URI}' targets bucket root. Syncing with --delete-unmatched-destination-objects must be scoped to a subpath prefix." >&2
		exit 1
		;;
	gs://*/*/*) ;;
	*)
		echo "ERROR: DEST_URI '${DEST_URI}' targets bucket root. Syncing with --delete-unmatched-destination-objects must be scoped to a subpath prefix." >&2
		exit 1
		;;
	esac

	resolved_src=$(fetch_git_source "$SRC_PATH" "$REPO_URL" "$REPO_REF")

	if [ ! -d "$resolved_src" ]; then
		echo "ERROR: Source directory not found: $resolved_src (configured source_path: $SRC_PATH)" >&2
		exit 1
	fi

	if [ -n "$EXCLUDE_REGEX" ]; then
		echo "--> Synchronizing Directory: $resolved_src -> $DEST_URI (excluding: $EXCLUDE_REGEX)"
		gcloud storage rsync -r --delete-unmatched-destination-objects -x "$EXCLUDE_REGEX" "$resolved_src" "$DEST_URI"
	else
		echo "--> Synchronizing Directory: $resolved_src -> $DEST_URI"
		gcloud storage rsync -r --delete-unmatched-destination-objects "$resolved_src" "$DEST_URI"
	fi

# 2. Process Individual File
elif [ "$MODE" = "file" ]; then
	resolved_src=$(fetch_git_source "$SRC_PATH" "$REPO_URL" "$REPO_REF")

	if [ ! -f "$resolved_src" ]; then
		echo "ERROR: Source file not found: $resolved_src (configured source_path: $SRC_PATH)" >&2
		exit 1
	fi

	echo "--> Uploading File: $resolved_src -> $DEST_URI"
	gcloud storage cp "$resolved_src" "$DEST_URI"
else
	echo "ERROR: Invalid or missing MODE: '$MODE' (must be 'directory' or 'file')" >&2
	exit 1
fi
