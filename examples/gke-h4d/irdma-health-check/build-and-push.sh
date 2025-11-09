#!/bin/bash
# Copyright 2025 Google LLC
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

set -e
cd "$(dirname "$0")"

# EDIT THIS: Your Google Cloud project ID
PROJECT_ID=MY-GCP-PROJECT
# EDIT THIS: The name of the image to build
IMAGE_NAME="irdma-health-check"
# EDIT THIS: The image tag
IMAGE_TAG="v1.0.0"

REGION="us-central1"

IMAGE_URI="${REGION}-docker.pkg.dev/${PROJECT_ID}/h4d/${IMAGE_NAME}:${IMAGE_TAG}"

echo "Building and pushing image: ${IMAGE_URI}"

docker build -t "${IMAGE_URI}" .
docker push "${IMAGE_URI}"

echo "Image pushed successfully."
