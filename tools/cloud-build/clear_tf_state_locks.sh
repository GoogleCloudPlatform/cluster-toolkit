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

# Usage: clear_tf_state_locks.sh <test_config.yml|blueprint.yaml> <deployment_name>
# Deletes leaked Terraform .tflock files for a single deployment without
# walking the bucket root (gs://<bucket>/*/... takes 12-25 min).
set -uo pipefail

TEST_CONFIG=${1:-}
DEPLOYMENT_NAME=${2:-}

if [ -z "$TEST_CONFIG" ] || [ -z "$DEPLOYMENT_NAME" ]; then
	echo "Usage: clear_tf_state_locks.sh <test_config.yml|blueprint.yaml> <deployment_name>" >&2
	exit 1
fi

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
BUCKET=${TF_STATE_BUCKET:-daily-tests-tf-state}
WORKSPACE=${WORKSPACE:-$(cd -- "$SCRIPT_DIR/../.." &>/dev/null && pwd)}

if grep -q '^[[:space:]]*blueprint_name:' "$TEST_CONFIG" 2>/dev/null; then
	bp_yaml="$TEST_CONFIG"
else
	bp_yaml=$(sed -n 's/^[[:space:]]*blueprint_yaml:[[:space:]]*//p' "$TEST_CONFIG" | head -n 1 | sed 's/[[:space:]]*#.*//;s/[[:space:]]*$//' | tr -d "\"'")
	bp_yaml=${bp_yaml//\{\{ workspace \}\}/$WORKSPACE}
fi
if [ -z "$bp_yaml" ] || [ ! -f "$bp_yaml" ]; then
	echo "WARNING: cannot resolve blueprint_yaml from $TEST_CONFIG; skipping tflock cleanup."
	exit 0
fi

bp_name=$(sed -n 's/^[[:space:]]*blueprint_name:[[:space:]]*//p' "$bp_yaml" | head -n 1 | sed 's/[[:space:]]*#.*//;s/[[:space:]]*$//' | tr -d "\"'")
if [ -z "$bp_name" ]; then
	echo "WARNING: no blueprint_name in $bp_yaml; skipping tflock cleanup."
	exit 0
fi

echo "Clearing leaked locks under gs://$BUCKET/$bp_name/$DEPLOYMENT_NAME/ ..."
if ! rm_out=$(timeout 60 gcloud storage rm -r "gs://$BUCKET/$bp_name/$DEPLOYMENT_NAME/**.tflock" 2>&1); then
	if ! printf '%s\n' "$rm_out" | grep -qi "matched no objects"; then
		echo "WARNING: failed to clear state locks: $rm_out" >&2
	fi
fi
