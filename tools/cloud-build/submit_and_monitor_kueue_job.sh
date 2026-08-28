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

set -eo pipefail

TEST_NAME=$1
BUILD_ID=$2

if [ -z "$TEST_NAME" ]; then
	echo "Usage: $0 <test-name>" >&2
	exit 1
fi

gcloud container clusters get-credentials test-kueue-cluster --region=us-central1
BUILD_ID_SHORT=$(echo "$BUILD_ID" | cut -c1-6)

# Read the actual job name from the manifest to handle cases where tests use abbreviated names (e.g. crd)
JOB_NAME=$(grep -m 1 -E '^ *name:' /workspace/job.yaml | awk '{print $2}')
if [ -z "$JOB_NAME" ]; then
	# Fallback if grep fails
	JOB_NAME="${TEST_NAME}-${BUILD_ID_SHORT}"
fi

# Cloud Build trap: If the Cloud Build step itself receives a cancellation signal,
# delete the GKE job so the pod receives a SIGTERM and performs infrastructure cleanup.
cleanup_cb() {
	echo ""
	echo "=========================================================================="
	echo "PIPELINE CANCELLED: The Cloud Build step received a termination signal."
	echo "Deleting GKE Kueue Job ($JOB_NAME) to force the Pod to clean itself up!"
	echo "=========================================================================="
	kubectl delete job "$JOB_NAME" -n default || true
	exit 1
}
trap cleanup_cb SIGTERM SIGINT

MAX_RETRIES=3
RETRY_DELAY=300
ATTEMPT=1

while true; do
	echo "=== ATTEMPT $ATTEMPT: Submitting Kueue Job ==="
	kubectl apply -f /workspace/job.yaml

	set +e
	(
		bash tools/cloud-build/monitor_kueue_job.sh \
			test-kueue-cluster \
			us-central1 \
			"$JOB_NAME" \
			default | tee /workspace/job_logs.txt
		exit "${PIPESTATUS[0]}"
	) &
	MONITOR_PID=$!
	wait $MONITOR_PID
	EXIT_CODE=$?
	set -e

	if [ $EXIT_CODE -eq 0 ]; then
		echo "Job succeeded!"
		break
	fi

	# Check if the failure was specifically due to a lack of GCP zone capacity.
	# If so, retry. If it's a real error (like a terraform syntax error), fail immediately.
	if bash tools/cloud-build/check_retriable_error.sh /workspace/job_logs.txt; then
		echo "WARNING: Retriable error detected. Kueue Job has already been deleted. Retrying in $RETRY_DELAY seconds..."
	else
		echo "ERROR: Test failed due to an actual error (not zone capacity). Failing pipeline." >&2
		exit 1
	fi

	if [ $ATTEMPT -ge $MAX_RETRIES ]; then
		echo "ERROR: Job failed to find zone capacity after $MAX_RETRIES attempts." >&2
		exit 1
	fi

	sleep $RETRY_DELAY
	ATTEMPT=$((ATTEMPT + 1))
done
