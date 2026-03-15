#!/bin/bash
# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

TRIGGER_BUILD_CONFIG_PATH="$1"

echo "$TRIGGER_BUILD_CONFIG_PATH"

MATCHING_BUILDS=$(gcloud builds list --ongoing --format 'value(id)' --filter="substitutions.TRIGGER_BUILD_CONFIG_PATH=\"$TRIGGER_BUILD_CONFIG_PATH\"")
MATCHING_COUNT=$(echo "$MATCHING_BUILDS" | wc -w)

if [ "$MATCHING_COUNT" -gt 1 ]; then
 # Allow multiple builds if it's an 'onspot' configuration and not a daily test
 if [[ "${_TEST_PREFIX}" != "daily-" && "$TRIGGER_BUILD_CONFIG_PATH" == *"onspot"* ]]; then
  echo "Found more than 1 matching running build(s), but allowing to continue as it is an 'onspot' non-daily build."
  echo "$MATCHING_BUILDS"
  exit 0
 fi

 echo "Found more than 1 matching running build(s):"
 echo "$MATCHING_BUILDS"
 exit 1
fi

echo "No other matching running builds found (or only one)."
exit 0