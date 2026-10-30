#!/bin/bash
# Copyright 2026 Google LLC
# ... (Apache 2.0 header)

# Usage: clear_tf_state_locks.sh <test_config.yml> <deployment_name>
# Deletes leaked Terraform .tflock files for a single deployment without
# walking the bucket root (gs://<bucket>/*/... takes 12-25 min).
set -uo pipefail

TEST_CONFIG=${1:-}
DEPLOYMENT_NAME=${2:-}

if [ -z "$TEST_CONFIG" ] || [ -z "$DEPLOYMENT_NAME" ]; then
	echo "Usage: clear_tf_state_locks.sh <test_config.yml> <deployment_name>" >&2
	exit 1
fi

BUCKET=${TF_STATE_BUCKET:-daily-tests-tf-state}
WORKSPACE=${WORKSPACE:-/workspace}

bp_yaml=$(sed -n 's/blueprint_yaml:[[:space:]]*//p' "$TEST_CONFIG" | head -1 | tr -d "\"'")
bp_yaml=${bp_yaml//\{\{ workspace \}\}/$WORKSPACE}
if [ -z "$bp_yaml" ] || [ ! -f "$bp_yaml" ]; then
	echo "WARNING: cannot resolve blueprint_yaml from $TEST_CONFIG; skipping tflock cleanup."
	exit 0
fi

bp_name=$(sed -n 's/blueprint_name:[[:space:]]*//p' "$bp_yaml" | head -1 | tr -d "\"'")
if [ -z "$bp_name" ]; then
	echo "WARNING: no blueprint_name in $bp_yaml; skipping tflock cleanup."
	exit 0
fi

echo "Clearing leaked locks under gs://$BUCKET/$bp_name/$DEPLOYMENT_NAME/ ..."
gcloud storage rm "gs://$BUCKET/$bp_name/$DEPLOYMENT_NAME/**.tflock" 2>/dev/null || true
