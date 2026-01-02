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

# Checks if a tag key exists and returns the ID or an empty string
set -eo pipefail

eval "$(jq -r '@sh "PARENT=\(.parent) SHORT_NAME=\(.short_name)"')"

ID=$(gcloud resource-manager tags keys list --parent="$PARENT" --format="value(name)" --filter="shortName=$SHORT_NAME" 2>/dev/null || echo "")

jq -n --arg id "$ID" '{"id": $id}'
