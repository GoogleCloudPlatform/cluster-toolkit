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

set -e

TRIGGER_BUILD_CONFIG_PATH="$1"

# Skip the matching build check for onspot PR builds.
# PR builds in Cloud Build for GitHub are checked out in detached HEAD
# and fetch a specific PR ref like refs/pull/123/head.
if [[ "$TRIGGER_BUILD_CONFIG_PATH" == *"onspot"* ]]; then
    echo "DEBUG: Detected 'onspot' in config path: $TRIGGER_BUILD_CONFIG_PATH"
    
    # In Google Cloud Build GitHub App triggers, PRs are fetched into FETCH_HEAD.
    # The output usually contains 'refs/pull/'.
    if git rev-parse --git-dir > /dev/null 2>&1; then
        echo "DEBUG: Inside a git repository."
        
        # Check git show output
        if git show FETCH_HEAD 2>/dev/null | grep -q "refs/pull/"; then
            echo "Skipping matching build check for onspot PR build: $TRIGGER_BUILD_CONFIG_PATH (matched via git show FETCH_HEAD)"
            exit 0
        else
            echo "DEBUG: Did not find 'refs/pull/' in 'git show FETCH_HEAD'."
        fi
        
        # Check .git/FETCH_HEAD file
        if grep -q "refs/pull/" .git/FETCH_HEAD 2>/dev/null; then
            echo "Skipping matching build check for onspot PR build: $TRIGGER_BUILD_CONFIG_PATH (matched via .git/FETCH_HEAD file)"
            exit 0
        else
            echo "DEBUG: Did not find 'refs/pull/' in '.git/FETCH_HEAD' file."
            echo "DEBUG: Contents of .git/FETCH_HEAD:"
            cat .git/FETCH_HEAD || echo "DEBUG: Could not read .git/FETCH_HEAD"
        fi
        
        # Checking to see what branch/ref we are currently on just in case
        echo "DEBUG: Current HEAD reference:"
        git log -1 --format="%H %D" || true
    else
        echo "DEBUG: Not inside a git repository."
    fi
else
    echo "DEBUG: 'onspot' not found in config path: $TRIGGER_BUILD_CONFIG_PATH"
fi

echo "DEBUG: Checking for running builds matching trigger: $TRIGGER_BUILD_CONFIG_PATH"

MATCHING_BUILDS=$(gcloud builds list --ongoing --format 'value(id)' --filter="substitutions.TRIGGER_BUILD_CONFIG_PATH=\"$TRIGGER_BUILD_CONFIG_PATH\"")
MATCHING_COUNT=$(echo "$MATCHING_BUILDS" | wc -w)

echo "DEBUG: MATCHING_BUILDS output: '$MATCHING_BUILDS'"
echo "DEBUG: MATCHING_COUNT: $MATCHING_COUNT"

if [ "$MATCHING_COUNT" -gt 1 ]; then
	echo "Found more than 1 matching running build(s):"
	echo "$MATCHING_BUILDS"
	exit 1
fi

echo "No other matching running builds found (or only one)."
exit 0