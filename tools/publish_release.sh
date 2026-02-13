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

# bash tools/publish_release.sh <release_name> <release_tag>
# bash tools/publish_release.sh

set -euo pipefail

BUCKET_PATH="gs://oss-ext-gate-prod-projects-bucket/cluster-toolkit/githubreleases/manifests/rel.json"

echo "Fetching latest tags from origin..."

if ! git fetch origin --tags; then
	echo "ERROR: Failed to fetch tags from remote repository."
	exit 1
fi

RELEASE_TAG="$(git tag --sort=-v:refname | head -n 1)"

if [[ -z "${RELEASE_TAG}" ]]; then
	echo "ERROR: No tags found in repository."
	exit 1
fi

RELEASE_NAME="${RELEASE_TAG}"

echo "Latest cluster-toolkit version detected:"
echo "Release Name : ${RELEASE_NAME}"
echo "Release Tag  : ${RELEASE_TAG}"

TMP_FILE="$(mktemp)"
trap 'rm -f "${TMP_FILE}"' EXIT

cat >"${TMP_FILE}" <<EOF
{
  "publish_all": true,
  "registry_config": {
    "github_releases": {
      "name": "${RELEASE_NAME}",
      "tag": "${RELEASE_TAG}"
    }
  }
}
EOF

echo "Uploading release manifest to GCS..."

if ! gcloud storage cp "${TMP_FILE}" "${BUCKET_PATH}"; then
	echo "ERROR: Failed to upload release manifest to bucket."
	exit 1
fi

echo "Release publish process finished successfully."
