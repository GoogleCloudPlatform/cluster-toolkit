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

# Defaults
CONFIG_PATH=""
OUTPUT_FILE="${OUTPUT_FILE:-/workspace/sequential_test_env.sh}"
SLEEP_INTERVAL=120
MAX_WAIT_SECONDS=7200 # 2 hours default

# Parse arguments
while [[ "$#" -gt 0 ]]; do
    case $1 in
        --config) CONFIG_PATH="$2"; shift 2 ;;
        *) TRIGGER_BUILD_CONFIG_PATH="$1"; shift ;;
    esac
done

if [ -z "$TRIGGER_BUILD_CONFIG_PATH" ]; then
    echo "Error: <trigger_build_config_path> is not provided."
    echo "--- DEBUG: Available Substitutions ---"
    if [ -n "$BUILD_ID" ]; then
         gcloud builds describe "$BUILD_ID" --format='value(substitutions)' || echo "gcloud describe failed"
    else
         echo "BUILD_ID env var is missing. Cannot describe build."
    fi
    echo "---------------------------------------"
    echo "Usage: $0 [--config <config_path>] <trigger_build_config_path>"
    exit 1
fi

echo "Trigger Build Config Path: $TRIGGER_BUILD_CONFIG_PATH"

# Get current build ID to exclude
CURRENT_BUILD_ID=$BUILD_ID
echo "Current Build ID: $CURRENT_BUILD_ID"

check_builds() {
    # Filter for ongoing builds with same TRIGGER_BUILD_CONFIG_PATH
    # Exclude CURRENT_BUILD_ID
    ALL_MATCHING=$(gcloud builds list --ongoing --format 'value(id)' --filter="substitutions.TRIGGER_BUILD_CONFIG_PATH=\"$TRIGGER_BUILD_CONFIG_PATH\"")
    
    # Filter out current build ID
    OTHER_MATCHING=""
    for id in $ALL_MATCHING; do
        if [ "$id" != "$CURRENT_BUILD_ID" ]; then
            OTHER_MATCHING="$OTHER_MATCHING $id"
        fi
    done
    
    echo "$OTHER_MATCHING"
}

WAIT_START=$(date +%s)

while true; do
    OTHER_BUILDS=$(check_builds)
    OTHER_COUNT=$(echo "$OTHER_BUILDS" | wc -w)
    
    if [ "$OTHER_COUNT" -eq 0 ]; then
        echo "No other concurrent builds found. Proceeding."
        break
    fi
    
    
    NOW=$(date +%s)
    ELAPSED=$((NOW - WAIT_START))
    if [ "$ELAPSED" -gt "$MAX_WAIT_SECONDS" ]; then
        echo "Timeout reached while waiting for other builds to finish ($MAX_WAIT_SECONDS seconds)."
        exit 1
    fi
    
    echo "Other build(s) running: $OTHER_BUILDS. Waiting $SLEEP_INTERVAL seconds... (Elapsed: ${ELAPSED}s)"
    sleep "$SLEEP_INTERVAL"
done

# Handle config.yml and network name
if [ -n "$CONFIG_PATH" ]; then
    if [ ! -f "$CONFIG_PATH" ]; then
        echo "Config file not found: $CONFIG_PATH"
        exit 1
    fi
    
    TEST_NAME=$(grep -E "^test_name:" "$CONFIG_PATH" | head -n 1 | awk '{print $2}' | tr -d '"' | tr -d "'")
    if [ -z "$TEST_NAME" ]; then
        echo "Could not extract test_name from $CONFIG_PATH"
        exit 1
    fi
    
    # Generate a safe network name (convert to lower case, replace invalid chars with hyphens)
    # Valid: lowercase letters, numbers, hyphens. Max 63 chars.
    SAFE_NETWORK_NAME=$(echo "$TEST_NAME" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9-]/-/g' | sed 's/^-//' | sed 's/-$//' | cut -c 1-63)
    
    echo "Test Name: $TEST_NAME"
    echo "Generated Network Name: $SAFE_NETWORK_NAME"
    
    echo "export NETWORK_NAME=\"$SAFE_NETWORK_NAME\"" > "$OUTPUT_FILE"
    echo "Environment variables written to $OUTPUT_FILE"
fi

exit 0
