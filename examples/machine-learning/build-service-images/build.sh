#!/bin/bash
# Copyright 2025 Google LLC
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

set -e -o pipefail

if [[ $# -ne 3 ]]; then
	echo "Usage: build.sh <image> <family_suffix> <project_id>"
	exit 1
fi

readonly RECIPE=$1
readonly SUFFIX="$2"
readonly PROJECT="$3"

if [[ ! -d "$RECIPE" ]]; then
	echo "The '$RECIPE' does not exist. Available recepies: common, a3m"
	exit 1
fi

readonly FAMILY="$RECIPE-$SUFFIX"
LONG_DEPLOYMENT_NAME="$USER-roll-$FAMILY"
readonly DEPLOYMENT_NAME="${LONG_DEPLOYMENT_NAME:0:31}"

readonly OUT_DIR="/tmp/build_$FAMILY"

echo "Building image in family: $FAMILY"
gcluster deploy "$RECIPE"/blueprint.yaml \
	--deployment-file "shared.yaml" \
	--vars deployment_name="$DEPLOYMENT_NAME" \
	--vars family="$FAMILY" \
	--vars project_id="$PROJECT" \
	--out="$OUT_DIR" \
	--auto-approve \
	--skip-validators="test_project_exists,test_apis_enabled" \
	--force

echo "Checking if the image was created successfully."
gcloud compute images describe-from-family "$FAMILY" --project="$PROJECT"

echo "Destroying supporting infrastructure at $OUT_DIR"
gcluster destroy "$OUT_DIR/$DEPLOYMENT_NAME" --auto-approve

echo "Successfully built image in family: $FAMILY"
