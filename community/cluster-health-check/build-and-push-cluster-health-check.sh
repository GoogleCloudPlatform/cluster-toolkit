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

set -e -u -o pipefail

usage() {
	echo >&2 "Usage: bash build-and-push-cluster-health-check.sh -p <PROJECT_ID> -r <REPO_NAME> -i <IMAGE_NAME> [-l <REGION>] [-v <VERSION>]"
	echo >&2 "This script builds a Docker image and pushes it to Google Artifact Registry."
	echo >&2 ""
	echo >&2 "Options:"
	echo >&2 "  -p    Your Google Cloud Project ID."
	echo >&2 "  -r    The Artifact Registry repository name."
	echo >&2 "  -i    The name for the Docker image."
	echo >&2 "  -l    (Optional) The Artifact Registry region. Defaults to 'us-central1'."
	echo >&2 "  -v    (Optional) The version tag for the image. Defaults to YYYY-MM-DD."
	echo >&2 "  -h    Display this help message."
	echo >&2 ""
	echo >&2 "Example (default region):"
	echo >&2 "  bash build-and-push-cluster-health-check.sh -p gpu-test-project -r cluster-health-check-repo -i cluster-health-check"
	echo >&2 ""
	echo >&2 "Example (specific region):"
	echo >&2 "  bash build-and-push-cluster-health-check.sh -p gpu-test-project -r cluster-health-check-repo -i cluster-health-check -l us-east4"
	exit 1
}

PROJECT=""
REPO=""
IMAGE=""
REGION="us-central1"
VERSION=""

while getopts ":p:r:i:l:v:h" opt; do
	case ${opt} in
	p)
		PROJECT=$OPTARG
		;;
	r)
		REPO=$OPTARG
		;;
	i)
		IMAGE=$OPTARG
		;;
	l)
		REGION=$OPTARG
		;;
	v)
		VERSION=$OPTARG
		;;
	h)
		usage
		;;
	\?)
		echo "Invalid Option: -$OPTARG" >&2
		usage
		;;
	:)
		echo "Invalid Option: -$OPTARG requires an argument." >&2
		usage
		;;
	esac
done

if [[ -z "${PROJECT}" ]] || [[ -z "${REPO}" ]] || [[ -z "${IMAGE}" ]]; then
	echo "Error: Missing required arguments." >&2
	usage
fi

if [[ -z "${VERSION}" ]]; then
	VERSION=$(date +%Y-%m-%d)
fi
REMOTE_DESTINATION="${REGION}-docker.pkg.dev/${PROJECT}/${REPO}/${IMAGE}"

echo "================================================="
echo "Configuration received:"
echo "  Project ID:       ${PROJECT}"
echo "  Repository Name:  ${REPO}"
echo "  Image Name:       ${IMAGE}"
echo "  Generated Version:  ${VERSION}"
echo "  Remote Destination: ${REMOTE_DESTINATION}"
echo "================================================="

set -x

echo "Building image ${IMAGE}:${VERSION}"

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)

echo "Vendoring Go dependencies..."
(cd "${SCRIPT_DIR}" && go mod vendor)

docker buildx build --platform linux/amd64,linux/arm64 \
	-t "${REMOTE_DESTINATION}:${VERSION}" \
	-t "${REMOTE_DESTINATION}:latest" \
	--target dcgm-healthcheck \
	--push \
	-f "${SCRIPT_DIR}/Dockerfile" \
	"${SCRIPT_DIR}"
