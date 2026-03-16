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

# jq is required to generate JSON safely for the release manifest.
# Ensure jq is installed on your system before running this script.
# bash tools/publish_release.sh <release_name> <release_tag>
# bash tools/publish_release.sh

set -euo pipefail

BUCKET_PATH="gs://oss-exit-gate-prod-projects-bucket/cluster-toolkit/githubreleases/manifests/rel.json"

# Check if jq is installed
if ! command -v jq >/dev/null 2>&1; then
	echo "ERROR: jq is required but not installed." >&2
	exit 1
fi

if [[ $# -eq 2 ]]; then
	RELEASE_NAME="$1"
	RELEASE_TAG="$2"

elif [[ $# -eq 0 ]]; then
	echo "Fetching latest tags from origin..."

	if ! git fetch origin --tags; then
		echo "ERROR: Failed to fetch tags from remote repository." >&2
		exit 1
	fi

	RELEASE_TAG="$(git tag --sort=-v:refname | head -n 1)"
	RELEASE_NAME="${RELEASE_TAG}"

else
	echo "ERROR: Invalid number of arguments." >&2
	echo "Usage: $0 [<release_name> <release_tag>]" >&2
	exit 1
fi

if [[ -z "${RELEASE_TAG}" || -z "${RELEASE_NAME}" ]]; then
	echo "ERROR: Release tag and name cannot be empty." >&2
	exit 1
fi

echo "Latest cluster-toolkit version detected:"
echo "Release Name : ${RELEASE_NAME}"
echo "Release Tag  : ${RELEASE_TAG}"

TMP_FILE="$(mktemp)"
trap 'rm -f "${TMP_FILE}"' EXIT

jq -n \
	--arg name "$RELEASE_NAME" \
	--arg tag "$RELEASE_TAG" \
	'{
  "publish_all": true,
  "registry_config": {
    "github_releases": {
      "name": $name,
      "tag": $tag
    }
  }
}' >"${TMP_FILE}"

echo "Uploading release manifest to GCS..."

if ! gcloud storage cp "${TMP_FILE}" "${BUCKET_PATH}"; then
	echo "ERROR: Failed to upload release manifest to bucket."
	exit 1
fi

echo "Release publish process finished successfully for tag ${RELEASE_TAG}"
