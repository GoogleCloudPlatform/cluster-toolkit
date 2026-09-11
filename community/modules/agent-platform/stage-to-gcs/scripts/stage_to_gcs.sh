#!/bin/bash
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
trap 'rm -rf "${TMP_GIT_DIR}"' EXIT

# Portable string hashing (Linux / macOS / BSD)
hash_string() {
	if command -v sha256sum >/dev/null 2>&1; then
		printf -- '%s' "$1" | sha256sum | cut -d" " -f1
	elif command -v shasum >/dev/null 2>&1; then
		printf -- '%s' "$1" | shasum -a 256 | cut -d" " -f1
	elif command -v md5sum >/dev/null 2>&1; then
		printf -- '%s' "$1" | md5sum | cut -d" " -f1
	elif command -v md5 >/dev/null 2>&1; then
		printf -- '%s' "$1" | md5 -q
	else
		printf -- '%s' "$1" | cksum | cut -d" " -f1
	fi
}

# Helper function to resolve source path (Local filesystem vs Git repository)
resolve_source() {
	local SRC_PATH="$1"
	local REPO_URL="$2"
	local REPO_REF="$3"

	if [ -n "$REPO_URL" ]; then
		local REPO_HASH
		REPO_HASH=$(hash_string "$REPO_URL-$REPO_REF")
		local CLONE_SUBDIR="$TMP_GIT_DIR/$REPO_HASH"
		if [ ! -d "$CLONE_SUBDIR" ]; then
			echo "--> Initializing sparse repository for: $REPO_URL (ref: $REPO_REF)..." >&2
			git init "$CLONE_SUBDIR" >&2
			git -C "$CLONE_SUBDIR" remote add origin "$REPO_URL" >&2
			git -C "$CLONE_SUBDIR" sparse-checkout init --cone >&2
			if ! git -C "$CLONE_SUBDIR" fetch --depth 1 --filter=blob:none origin "$REPO_REF" >/dev/null 2>&1; then
				echo "--> Direct shallow ref fetch failed; fetching commit graph for: $REPO_REF..." >&2
				git -C "$CLONE_SUBDIR" fetch --filter=blob:none origin >&2
			fi
		fi
		git -C "$CLONE_SUBDIR" sparse-checkout add --skip-checks "$SRC_PATH" >&2
		git -C "$CLONE_SUBDIR" checkout "$REPO_REF" >/dev/null 2>&1 || git -C "$CLONE_SUBDIR" checkout FETCH_HEAD >&2
		echo "$CLONE_SUBDIR/$SRC_PATH"
	else
		case "$SRC_PATH" in
		/*)
			echo "$SRC_PATH"
			return
			;;
		esac

		if [ -e "$SRC_PATH" ]; then
			echo "$SRC_PATH"
			return
		fi

		local CUR_DIR
		CUR_DIR="$(pwd -P)"
		while [ "$CUR_DIR" != "/" ] && [ -n "$CUR_DIR" ]; do
			if [ -e "$CUR_DIR/$SRC_PATH" ]; then
				echo "$CUR_DIR/$SRC_PATH"
				return
			fi
			CUR_DIR=$(dirname "$CUR_DIR")
		done

		echo "$SRC_PATH"
	fi
}

# 1. Process Directory Sync
if [ "$MODE" = "directory" ]; then
	resolved_src=$(resolve_source "$SRC_PATH" "$REPO_URL" "$REPO_REF")

	if [ ! -d "$resolved_src" ]; then
		echo "ERROR: Source directory not found: $resolved_src (configured source_path: $SRC_PATH)" >&2
		exit 1
	fi

	if [ -n "$EXCLUDE_REGEX" ]; then
		echo "--> Staging Directory: $resolved_src -> $DEST_URI (excluding: $EXCLUDE_REGEX)"
		gcloud storage rsync -r -x "$EXCLUDE_REGEX" "$resolved_src" "$DEST_URI"
	else
		echo "--> Staging Directory: $resolved_src -> $DEST_URI"
		gcloud storage rsync -r "$resolved_src" "$DEST_URI"
	fi

# 2. Process Individual File
elif [ "$MODE" = "file" ]; then
	resolved_src=$(resolve_source "$SRC_PATH" "$REPO_URL" "$REPO_REF")

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
