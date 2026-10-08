#!/bin/bash
# Copyright 2026 "Google LLC"
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

set -euo pipefail

DEST_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/cluster-toolkit-examples"
TIMESTAMP_FILE="$DEST_DIR/.last_updated"
FRESHNESS_TTL=86400 # 24 hours in seconds
BRANCH="${CLUSTER_TOOLKIT_BRANCH:-main}"

# 1. If running inside a local Cluster Toolkit repo, copy local examples/
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel 2>/dev/null || true)

# Sentinel check: confirm this is a real Cluster Toolkit repository
if [ -n "$REPO_ROOT" ] && [ -d "$REPO_ROOT/examples" ] && [ -d "$REPO_ROOT/modules/compute/gke-node-pool" ]; then
	echo "Using local Cluster Toolkit repository examples from $REPO_ROOT/examples..."
	mkdir -p "$DEST_DIR"
	rm -rf "$DEST_DIR/examples"
	cp -r "$REPO_ROOT/examples" "$DEST_DIR/examples"
	touch "$TIMESTAMP_FILE"
	echo "Done syncing local examples."
	exit 0
fi

# 2. Check cache freshness for standalone runs
if [ -f "$TIMESTAMP_FILE" ] && [ -d "$DEST_DIR/examples" ]; then
	LAST_MOD=$(stat -c %Y "$TIMESTAMP_FILE" 2>/dev/null || stat -f %m "$TIMESTAMP_FILE" 2>/dev/null || echo 0)
	NOW=$(date +%s)
	AGE=$((NOW - LAST_MOD))

	if [ "$AGE" -lt "$FRESHNESS_TTL" ]; then
		echo "Cached templates at $DEST_DIR are fresh (age: ${AGE}s < ${FRESHNESS_TTL}s)."
		exit 0
	else
		echo "Cached templates at $DEST_DIR are stale (age: ${AGE}s). Updating..."
	fi
fi

# 3. Pull latest branch cleanly from remote GitHub repo
echo "Fetching latest ClusterToolkit examples (${BRANCH}) to $DEST_DIR..."
mkdir -p "$DEST_DIR"
cd "$DEST_DIR"

if [ ! -d ".git" ]; then
	git init
	git remote add origin https://github.com/GoogleCloudPlatform/cluster-toolkit.git
	git config core.sparseCheckout true
	echo "examples/*" >.git/info/sparse-checkout
else
	git remote set-url origin https://github.com/GoogleCloudPlatform/cluster-toolkit.git 2>/dev/null || git remote add origin https://github.com/GoogleCloudPlatform/cluster-toolkit.git
fi

# Ensure clean directory state during sync; gracefully fallback to cache on network/remote failure
if git fetch --depth=1 origin "$BRANCH" && git reset --hard FETCH_HEAD; then
	touch "$TIMESTAMP_FILE"
	echo "Done fetching latest remote templates cleanly."
elif [ -d "$DEST_DIR/examples" ]; then
	echo "Warning: Failed to fetch remote templates. Falling back to existing cached templates."
	exit 0
else
	echo "Error: Failed to fetch remote templates and no local cache is available." >&2
	exit 1
fi
