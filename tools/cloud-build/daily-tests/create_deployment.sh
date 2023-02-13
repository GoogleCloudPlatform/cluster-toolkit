#!/bin/bash
# Copyright 2022 Google LLC
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

# Set variables to default if not already set
EXAMPLE_YAML=${EXAMPLE_YAML:-/workspace/examples/hpc-cluster-high-io.yaml}
PROJECT=${PROJECT:-hpc-toolkit-dev}
DEPLOYMENT_NAME=${DEPLOYMENT_NAME:-missing-deployment-name}
NETWORK=${NETWORK:-missing-network-name}
MAX_NODES=${MAX_NODES:-2}
TEST_NAME=${TEST_NAME:unnamed_test}
ALWAYS_RECOMPILE=${ALWAYS_RECOMPILE:-yes}
GHPC_DEV_BUCKET=${GHPC_DEV_BUCKET:-daily-tests-tf-state}

echo "Creating blueprint from ${EXAMPLE_YAML} in project ${PROJECT} for test ${TEST_NAME}"

## Add GCS Backend to example
echo "Adding GCS Backend to the yaml (bucket: ${GHPC_DEV_BUCKET})"
if ! grep -Fxq terraform_backend_defaults: "${EXAMPLE_YAML}"; then
	cat <<EOT >>"${EXAMPLE_YAML}"

terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: ${GHPC_DEV_BUCKET}
EOT
fi

## Build ghpc
cd "$ROOT_DIR" ||
	{
		echo "*** ERROR: failed to access root directory ${ROOT_DIR} when creating blueprint"
		exit 1
	}

if [[ $ALWAYS_RECOMPILE != "no" || ! -f ghpc ]]; then
	make
else
	echo "Skipping recompilation due to pre-existing ghpc binary and ALWAYS_RECOMPILE == 'no'"
fi

## Customize config yaml
sed -i "s/network_name: .*/network_name: ${NETWORK}/" "${EXAMPLE_YAML}" ||
	{
		echo "could not set network_name, may be using pre-existing-vpc"
	}
sed -i "s/max_node_count: .*/max_node_count: ${MAX_NODES}/" "${EXAMPLE_YAML}" ||
	{
		echo "could not set max_node_count"
	}

VARS="project_id=${PROJECT_ID},deployment_name=${DEPLOYMENT_NAME}"

## Create blueprint and create artifact
./ghpc create  -l ERROR "${EXAMPLE_YAML}" \
	--vars "${VARS}" ||
	{
		echo "could not write blueprint"
		exit 1
	}
tar -czf "${DEPLOYMENT_NAME}.tgz" "${DEPLOYMENT_NAME}" ||
	{
		echo "could not tarball blueprint"
		exit 1
	}

echo "Uploading deployment to gs://${GHPC_DEV_BUCKET}/${TEST_NAME}/${DEPLOYMENT_NAME}.tgz"
gsutil cp "${DEPLOYMENT_NAME}.tgz" "gs://${GHPC_DEV_BUCKET}/${TEST_NAME}/"
