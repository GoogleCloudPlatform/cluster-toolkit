#!/bin/bash
# Copyright 2021 Google LLC
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
BLUEPRINT_DIR=${BLUEPRINT_DIR:-blueprint}
BUILD_ID=${BUILD_ID:-undefined-build}
MAX_NODES=${MAX_NODES:-2}
echo "Creating blueprint from ${EXAMPLE_YAML} in project ${PROJECT}"

## Add GCS Backend to example
if ! grep -Fxq terraform_backend_defaults: "${EXAMPLE_YAML}";
then
  cat <<EOT >> "${EXAMPLE_YAML}"

terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: daily-tests-tf-state
EOT
fi

## Build ghpc
cd "$ROOT_DIR" || \
  { echo "*** ERROR: failed to access root directory ${ROOT_DIR} when creating blueprint"; exit 1; }
make

## Customize config yaml
sed -i "s/project_id: .*/project_id: ${PROJECT_ID}/"  "${EXAMPLE_YAML}" || \
     { echo "could not set project_id"; exit 1; }
sed -i "s/blueprint_name: .*/blueprint_name: ${BLUEPRINT_DIR}/" "${EXAMPLE_YAML}" || \
     { echo "could not set blueprint_name"; exit 1; }
sed -i "s/deployment_name: \(.*\)/deployment_name: \1-${BUILD_ID:0:6}/" "${EXAMPLE_YAML}" || \
     { echo "could not set deployment_name"; exit 1; }
sed -i "s/max_node_count: .*/max_node_count: ${MAX_NODES}/"  "${EXAMPLE_YAML}" || \
     { echo "could not set max_node_count"; exit 1; }

## Create blueprint and create artifact
./ghpc create -c "${EXAMPLE_YAML}"
tar -czf "${BLUEPRINT_DIR}.tgz" "${BLUEPRINT_DIR}"
