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

set -x
set -e

REGISTRY_URL=$1
IMAGE_NAME=$2
BUCKET=$3
INSTALLER=$4
FIXPACK=$5

BUILD_DIR="/tmp/sym-build"
mkdir -p $BUILD_DIR

cp /tmp/Symphony/Dockerfile.sym-compute $BUILD_DIR/Dockerfile.sym-compute
cp /tmp/Symphony/cloudbuild.yaml $BUILD_DIR/cloudbuild.yaml

cd $BUILD_DIR

FULL_IMAGE_NAME="${REGISTRY_URL}/${IMAGE_NAME}:latest"
OPERATOR_IMAGE_NAME="${REGISTRY_URL}/gcp-symphony-operator:latest"

# Get master VM service account and project ID from instance metadata
SA_EMAIL=$(curl -s -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/email)
PROJECT_ID=$(curl -s -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/project/project-id)

# Submit build to Cloud Build using the Master VM's service account
gcloud builds submit --config cloudbuild.yaml \
    --service-account="projects/${PROJECT_ID}/serviceAccounts/${SA_EMAIL}" \
    --substitutions=_BUCKET="${BUCKET}",_INSTALLER="${INSTALLER}",_FIXPACK="${FIXPACK}",_IMAGE="${FULL_IMAGE_NAME}",_OPERATOR_IMAGE="${OPERATOR_IMAGE_NAME}" \
    --timeout=30m

echo "Successfully built and pushed ${FULL_IMAGE_NAME} and ${OPERATOR_IMAGE_NAME}"

# Save the image name to a file
echo "${FULL_IMAGE_NAME}" > /tmp/Symphony/compute_image_name.txt
