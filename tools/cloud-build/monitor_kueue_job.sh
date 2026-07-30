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

set -o pipefail

if [ "$#" -ne 4 ]; then
	echo "Usage: $0 <cluster-name> <region> <job-name> <namespace>"
	exit 1
fi

CLUSTER_NAME="$1"
REGION="$2"
JOB_NAME="$3"
NAMESPACE="$4"

echo "Connecting to GKE cluster $CLUSTER_NAME in $REGION..."
gcloud container clusters get-credentials "$CLUSTER_NAME" --region="$REGION"

echo "Waiting for Job $JOB_NAME to be admitted by Kueue..."
while true; do
	if ! SUSPENDED=$(kubectl get job "$JOB_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.suspend}' 2>/dev/null); then
		echo "Error: Job $JOB_NAME not found yet or kubectl failed. Retrying..."
		sleep 5
		continue
	fi
	if [ "$SUSPENDED" = "false" ]; then
		echo "Job admitted by Kueue!"
		break
	fi
	echo "Job is currently suspended/queued. Waiting..."
	sleep 10
done

echo "Waiting for Pod to be initialized..."
POD_INIT_TIMEOUT=1200
START_TIME=$SECONDS
while true; do
	if [ $((SECONDS - START_TIME)) -gt $POD_INIT_TIMEOUT ]; then
		echo "Error: Pod initialization timed out after 20 minutes." >&2
		exit 1
	fi
	POD_NAME=$(kubectl get pods -n "$NAMESPACE" --selector=job-name="$JOB_NAME" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
	if [ -n "$POD_NAME" ]; then
		POD_PHASE=$(kubectl get pod "$POD_NAME" -n "$NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null)
		if [ "$POD_PHASE" = "Running" ] || [ "$POD_PHASE" = "Succeeded" ] || [ "$POD_PHASE" = "Failed" ]; then
			echo "Pod $POD_NAME is $POD_PHASE. Ready to stream logs."
			break
		fi
		echo "Pod $POD_NAME detected but is in phase: $POD_PHASE. Waiting for it to start running..."
	fi
	sleep 5
done

echo "Streaming logs from pod $POD_NAME..."
while true; do
	# Stream logs
	kubectl logs -f "$POD_NAME" -n "$NAMESPACE" -c runner
	# Allow the Kubernetes controller a few seconds to update the Job status
	# after the pod terminates. This prevents a race condition where we check
	# the status before it's updated and accidentally restart the log stream.
	sleep 5

	# Check if job is completed
	if ! JOB_STATUS=$(kubectl get job "$JOB_NAME" -n "$NAMESPACE" -o jsonpath='{.status.succeeded}{"|"}{.status.failed}' 2>/dev/null); then
		echo "Error: Job $JOB_NAME no longer exists or kubectl failed. Exiting." >&2
		exit 1
	fi
	IFS='|' read -r SUCCEEDED FAILED <<<"$JOB_STATUS"
	if [ "$SUCCEEDED" = "1" ] || [ "$FAILED" = "1" ]; then
		break
	fi
	echo "Logs disconnected but job is not finished. Reconnecting..."
	# If it actually disconnected mid-job, sleep a moment before reconnecting
	sleep 2
done

echo "Job finished execution. Fetching final status..."
SUCCEEDED=$(kubectl get job "$JOB_NAME" -n "$NAMESPACE" -o jsonpath='{.status.succeeded}' 2>/dev/null)
FAILED=$(kubectl get job "$JOB_NAME" -n "$NAMESPACE" -o jsonpath='{.status.failed}' 2>/dev/null)

echo "Job status: Succeeded=$SUCCEEDED, Failed=$FAILED"

echo "Cleaning up GKE Job resource..."
kubectl delete job "$JOB_NAME" -n "$NAMESPACE"

if [ "$SUCCEEDED" = "1" ]; then
	echo "GKE Kueue Job completed successfully."
	exit 0
else
	echo "GKE Kueue Job failed (or was aborted)."
	exit 1
fi
