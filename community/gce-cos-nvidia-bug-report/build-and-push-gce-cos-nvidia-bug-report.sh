#!/bin/bash
set -e -u -o pipefail

usage() {
  echo >&2 "Usage: bash build-and-push-gce-cos-nvidia-bug-report.sh -p <PROJECT_ID> -r <REPO_NAME> -i <IMAGE_NAME> [-l <REGION>]"
  echo >&2 "This script builds a Docker image and pushes it to Google Artifact Registry."
  echo >&2 ""
  echo >&2 "Options:"
  echo >&2 "  -p    Your Google Cloud Project ID."
  echo >&2 "  -r    The Artifact Registry repository name."
  echo >&2 "  -i    The name for the Docker image."
  echo >&2 "  -l    (Optional) The Artifact Registry region. Defaults to 'us-central1'."
  echo >&2 "  -h    Display this help message."
  echo >&2 ""
  echo >&2 "Example (default region):"
  echo >&2 "  bash build-and-push-gce-cos-nvidia-bug-report.sh -p gpu-test-project -r gce-cos-nvidia-bug-report-repo -i gce-cos-nvidia-bug-report"
  echo >&2 ""
  echo >&2 "Example (specific region):"
  echo >&2 "  bash build-and-push-gce-cos-nvidia-bug-report.sh -p gpu-test-project -r gce-cos-nvidia-bug-report-repo -i gce-cos-nvidia-bug-report -l us-east4"
  exit 1
}

PROJECT=""
REPO=""
IMAGE=""
REGION="us-central1"

while getopts ":p:r:i:l:h" opt; do
  case ${opt} in
    p )
      PROJECT=$OPTARG
      ;;
    r )
      REPO=$OPTARG
      ;;
    i )
      IMAGE=$OPTARG
      ;;
    l )
      REGION=$OPTARG
      ;;
    h )
      usage
      ;;
    \? )
      echo "Invalid Option: -$OPTARG" >&2
      usage
      ;;
    : )
      echo "Invalid Option: -$OPTARG requires an argument." >&2
      usage
      ;;
  esac
done

if [[ -z "${PROJECT}" ]] || [[ -z "${REPO}" ]] || [[ -z "${IMAGE}" ]]; then
  echo "Error: Missing required arguments." >&2
  usage
fi

VERSION=$(date +%Y-%m-%d)
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
docker build -t "${IMAGE}:${VERSION}" .


echo "Tagging image for Artifact Registry: ${REMOTE_DESTINATION}:${VERSION}"
docker tag "${IMAGE}:${VERSION}" "${REMOTE_DESTINATION}:${VERSION}"
docker tag "${IMAGE}:${VERSION}" "${REMOTE_DESTINATION}:latest"

echo "Pushing image ${REMOTE_DESTINATION}:${VERSION}"
docker push "${REMOTE_DESTINATION}:${VERSION}"
docker push "${REMOTE_DESTINATION}:latest"
