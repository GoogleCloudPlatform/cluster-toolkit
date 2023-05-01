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
NETWORK=${NETWORK:-missing-network-name}
MAX_NODES=${MAX_NODES:-2}
ALWAYS_RECOMPILE=${ALWAYS_RECOMPILE:-yes}
GHPC_DEV_BUCKET=${GHPC_DEV_BUCKET:-daily-tests-tf-state}

echo "Preping blueprint from ${EXAMPLE_YAML}"

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
