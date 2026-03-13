#!/bin/bash
# Copyright 2026 "Google LLC"

set -e

TRIGGER_BUILD_CONFIG_PATH="$1"
Test_P="$2"


echo "DEBUG: _TEST_PREFIX is '${_TEST_PREFIX}'"
echo "DEBUG: Test_P is '${Test_P}'"

# Define Boolean: True if _TEST_PREFIX is "daily-", False if empty (PR test)
IS_DAILY=false
[[ "${_TEST_PREFIX}" == "daily-" ]] && IS_DAILY=true


echo "Config: $TRIGGER_BUILD_CONFIG_PATH | Daily: $IS_DAILY"

# Fetch matching ongoing builds for this config path
MATCHING_BUILDS=$(gcloud builds list --ongoing --format 'value(id)' --filter="substitutions.TRIGGER_BUILD_CONFIG_PATH=\"$TRIGGER_BUILD_CONFIG_PATH\"")
MATCHING_COUNT=$(echo "$MATCHING_BUILDS" | wc -w)

if [ "$MATCHING_COUNT" -gt 1 ]; then
  
  # Logic: 
  # If it's a PR test (NOT daily) AND it's an 'onspot' build, we allow more than one.
  if [ "$IS_DAILY" = false ] && [[ "$TRIGGER_BUILD_CONFIG_PATH" == *"onspot"* ]]; then
    echo "Found $MATCHING_COUNT matching builds. Allowing multiple as this is an 'onspot' PR test."
    echo "$MATCHING_BUILDS"
    exit 0
  fi

  # Otherwise, block the build (Strictly 1 for Daily tests or non-onspot PRs)
  echo "Error: Multiple matching builds detected."
  echo "Daily Test: $IS_DAILY"
  echo "Matching Build IDs: $MATCHING_BUILDS"
  exit 1
fi

echo "Check passed: No conflicting builds found."
exit 0