#!/usr/bin/env bash
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

BASE_IMAGE="${1:-${BASE_IMAGE:-}}"
TARGET_IMAGE="${2:-${TARGET_IMAGE:-}}"
ENTRYPOINT="${3:-${ENTRYPOINT:-}}"
MAX_RETRIES="${4:-${MAX_RETRIES:-3}}"
RETRY_DELAY="${5:-${RETRY_DELAY:-10}}"

# Fall back to default repo paths and validate required environment variables
if [ -z "$BASE_IMAGE" ]; then
	if [ -z "${PROJECT_ID:-}" ]; then
		echo "ERROR: PROJECT_ID environment variable must be set when using default BASE_IMAGE." >&2
		exit 1
	fi
	BASE_IMAGE="us-central1-docker.pkg.dev/$PROJECT_ID/hpc-toolkit-repo/test-runner:latest"
fi

if [ -z "$TARGET_IMAGE" ]; then
	if [ -z "${PROJECT_ID:-}" ] || [ -z "${BUILD_ID:-}" ]; then
		echo "ERROR: PROJECT_ID and BUILD_ID environment variables must be set when using default TARGET_IMAGE." >&2
		exit 1
	fi
	TARGET_IMAGE="us-central1-docker.pkg.dev/$PROJECT_ID/hpc-toolkit-repo/test-runner:$BUILD_ID"
fi

ENTRYPOINT_LINE=""
if [ -n "$ENTRYPOINT" ]; then
	if [[ "$ENTRYPOINT" =~ ^\[.*\]$ ]]; then
		ENTRYPOINT_LINE="ENTRYPOINT $ENTRYPOINT"
	else
		mapfile -t ep_args < <(xargs -n 1 <<<"$ENTRYPOINT")
		printf -v formatted_args '"%s", ' "${ep_args[@]}"
		ENTRYPOINT_LINE="ENTRYPOINT [${formatted_args%, }]"
	fi
fi

docker build -t "$TARGET_IMAGE" -f- . <<EOF
FROM $BASE_IMAGE
${ENTRYPOINT_LINE}
COPY . /workspace
WORKDIR /workspace
EOF

for ((i = 1; i <= MAX_RETRIES; i++)); do
	echo "Push attempt $i of $MAX_RETRIES for \"$TARGET_IMAGE\"..."
	if docker push "$TARGET_IMAGE"; then
		echo "Image pushed successfully!"
		exit 0
	fi

	if [ "$i" -eq "$MAX_RETRIES" ]; then
		echo "Image push failed after $MAX_RETRIES attempts. (Check https://status.cloud.google.com/ for outages)" >&2
		exit 1
	fi

	echo "Image push failed, retrying in ${RETRY_DELAY}s..." >&2
	sleep "$RETRY_DELAY"
done
