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

IMAGE=${IMAGE:-"us-central1-docker.pkg.dev/$PROJECT_ID/hpc-toolkit-repo/test-runner:$BUILD_ID"}
MAX_RETRIES=5
RETRY_DELAY=10
ATTEMPT=1

echo "Starting cleanup for image: $IMAGE"

while [ "$ATTEMPT" -le "$MAX_RETRIES" ]; do
	echo "Attempt $ATTEMPT of $MAX_RETRIES..."

	if gcloud artifacts docker images delete "$IMAGE" --quiet; then
		echo "Image successfully deleted."
		exit 0
	fi

	if [ "$ATTEMPT" -lt "$MAX_RETRIES" ]; then
		echo "Deletion failed. Retrying in $RETRY_DELAY seconds..."
		sleep $RETRY_DELAY
	fi

	ATTEMPT=$((ATTEMPT + 1))
done

echo "Failed to delete image after $MAX_RETRIES attempts."
echo "Image will be handled by Artifact Registry background cleanup policies."
exit 1
