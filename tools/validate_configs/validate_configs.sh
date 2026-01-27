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

set -e -o pipefail

run_test() {
	example=$1
	if [ -n "$2" ]; then
		deployment_args=("-d" "${cwd}/$2")
	else
		deployment_args=()
	fi
	tmpdir="$(mktemp -d)"
	exampleFile=$(basename "$example")
	# Generates a short deployment name to satisfy constraints (e.g. 10 char limit for slurm_cluster_name)
	# Logic: takes first 4 chars of filename (w/o dashes) + first 5 chars of random suffix
	base_clean=${exampleFile%.yaml}
	base_clean=${base_clean//-/}
	suffix=$(basename "${tmpdir##*.}")
	suffix=${suffix,,}
	DEPLOYMENT="${base_clean:0:4}${suffix:0:5}"
	PROJECT="invalid-project"
	# Skip test_deployment_variable_not_used to allow passing a broad set of mock variables
	VALIDATORS_TO_SKIP="test_project_exists,test_apis_enabled,test_region_exists,test_zone_exists,test_zone_in_region,test_deployment_variable_not_used"
	GHPC_PATH="${cwd}/ghpc"
	BP_PATH="${cwd}/${example}"
	# Cover the three possible starting sequences for local sources: ./ ../ /
	LOCAL_SOURCE_PATTERN='source:\s\+\(\./\|\.\./\|/\)'

	echo "testing ${example} in ${tmpdir}"

	# Only run from the repo directory if there are local modules, otherwise
	# run the test from the test directory using the installed gcluster binary.
	if grep -q "${LOCAL_SOURCE_PATTERN}" "${cwd}/${example}"; then
		cd "${cwd}"
	else
		cd "${tmpdir}"
	fi

	# Predefine common variables
	PREDEFINED_VARS="project_id=${PROJECT},deployment_name=${DEPLOYMENT},region=us-central1,zone=us-central1-a,zones=['us-central1-a'],authorized_cidr=0.0.0.0/0"
	PREDEFINED_VARS+=",health_check_schedule='0 0 * * 0',number_of_vms=2,static_node_count=2,slurm_cluster_name=testname,a3_partition_name=testname"

	# Generate all other mock variables dynamically
	# We use python to parse the blueprint and fill in the rest
	DEPLOYMENT_FILE=""
	if [ -n "$2" ]; then
		DEPLOYMENT_FILE="${cwd}/$2"
	fi
	MOCK_VARS=$("${cwd}/tools/validate_configs/get_mock_vars.py" "${cwd}/${example}" "${PREDEFINED_VARS}" "${DEPLOYMENT_FILE}")
	echo "DEBUG: MOCK_VARS for ${exampleFile}: ${MOCK_VARS}"

	${GHPC_PATH} create "${BP_PATH}" "${deployment_args[@]}" -l ERROR \
		--skip-validators="${VALIDATORS_TO_SKIP}" \
		--vars="${MOCK_VARS}" >/dev/null ||
		{
			echo "*** ERROR: error creating deployment with gcluster for ${exampleFile}"
			exit 1
		}
	if grep -q "${LOCAL_SOURCE_PATTERN}" "${cwd}/${example}"; then
		mv "${DEPLOYMENT}" "${tmpdir}"
	fi
	cd "${tmpdir}"/"${DEPLOYMENT}" || {
		echo "*** ERROR: can't cd into the deployment folder ${DEPLOYMENT}"
		exit 1
	}
	for folder in */; do
		cd "$folder"
		# Use mapfile (readarray) for cleaner array reading (Bash 4.0+)
		mapfile -t pkrdirs < <(find . -name "*.pkr.hcl" -printf '%h\n' | sort -u)
		if [ -f 'main.tf' ]; then
			tfpw=$(pwd)
			terraform init -no-color -backend=false >"${exampleFile}.init" ||
				{
					echo "*** ERROR: terraform init failed for ${example}, logs in ${tfpw}"
					exit 1
				}
			terraform validate -no-color >"${exampleFile}.plan" ||
				{
					echo "*** ERROR: terraform validate failed for ${example}, logs in ${tfpw}"
					exit 1
				}
		elif [ ${#pkrdirs[@]} -gt 0 ]; then
			for pkrdir in "${pkrdirs[@]}"; do
				packer validate -syntax-only "${pkrdir}" >/dev/null ||
					{
						echo "*** ERROR: packer validate failed for ${example}"
						exit 1
					}
			done
		else
			echo "neither packer nor terraform found in folder ${DEPLOYMENT}/${folder}. Skipping."
		fi
		cd .. # back to deployment folder
	done
	cd ..
	rm -rf "${DEPLOYMENT}" || {
		echo "*** ERROR: could not remove deployment folder from $(pwd)"
		exit 1
	}
	cd "${cwd}"
	rm -r "${tmpdir}"
}

check_background() {
	# "wait -n" is available since Bash 4.3. We assume a reasonably modern environment.
	if ! wait -n; then
		wait
		echo "*** ERROR: a test failed. Exiting with status 1."
		exit 1
	fi
}

CONFIGS=$(find examples/ community/examples/ tools/validate_configs/test_configs/ docs/tutorials/ docs/videos/build-your-own-blueprint/ -name "*.yaml" -type f -not -path "*/build-service-images/*")

# Exclude blueprints that use v5 modules.
declare -A EXCLUDE_EXAMPLE
EXCLUDE_EXAMPLE["tools/validate_configs/test_configs/two-clusters-sql.yaml"]=
EXCLUDE_EXAMPLE["community/examples/sycomp/sycomp-storage.yaml"]=
EXCLUDE_EXAMPLE["community/examples/sycomp/sycomp-storage-ece.yaml"]=
EXCLUDE_EXAMPLE["community/examples/sycomp/sycomp-storage-slurm.yaml"]=
EXCLUDE_EXAMPLE["community/examples/sycomp/sycomp-storage-expansion.yaml"]=

cwd=$(pwd)
NPROCS=${NPROCS:-$(nproc)}
echo "Running tests in $NPROCS processes"
pids=()
for example in $CONFIGS; do
	if [[ ${EXCLUDE_EXAMPLE[$example]+_} ]]; then
		echo "Skipping example: $example"
		continue
	fi

	# Skip deployment files; they will be used by the blueprint loop
	if [[ "$example" == *"-deployment.yaml" || "$example" == *"/deployment.yaml" ]]; then
		continue
	fi
	# Skip if it doesn't look like a blueprint
	if ! grep -q "^blueprint_name:" "$example" && ! grep -q "^---" "$example"; then
		echo "Skipping non-blueprint: $example"
		continue
	fi
	# Even if it has ---, ensure it's not just a k8s manifest
	if grep -q "kind:" "$example" && ! grep -q "blueprint_name:" "$example"; then
		echo "Skipping manifest: $example"
		continue
	fi

	dir=$(dirname "$example")
	base=$(basename "$example" .yaml)

	# Handle cases like a3mega-slurm-gcsfuse-lssd-blueprint.yaml sharing a3mega-slurm-deployment.yaml
	# or blueprints named without '-blueprint' suffix.
	prefix=${base%-blueprint}
	deployment="$dir/${prefix}-deployment.yaml"

	if [ ! -f "$deployment" ]; then
		# Try to find any deployment file in the same directory if the specific one doesn't exist
		shopt -s nullglob
		possible_deployments=("$dir"/*-deployment.yaml)
		shopt -u nullglob
		if [ ${#possible_deployments[@]} -eq 1 ]; then
			d_base=$(basename "${possible_deployments[0]}" -deployment.yaml)
			if [[ "$base" == "$d_base"* ]]; then
				deployment="${possible_deployments[0]}"
			fi
		elif [ ${#possible_deployments[@]} -gt 1 ]; then
			for d in "${possible_deployments[@]}"; do
				d_base=$(basename "$d" -deployment.yaml)
				if [[ "$base" == "$d_base"* ]]; then
					deployment="$d"
					break
				fi
			done
		fi
	fi

	JNUM=$(jobs | wc -l)
	# echo "$JNUM jobs running"
	if [ "$JNUM" -ge "$NPROCS" ]; then
		check_background
	fi

	if [ -f "$deployment" ]; then
		run_test "$example" "$deployment" &
	else
		run_test "$example" &
	fi
	pids+=("$!")
done
JNUM=$(jobs | wc -l)
while [ "$JNUM" -gt 0 ]; do
	check_background
	JNUM=$(jobs | wc -l)
done

echo "All configs have been validated successfully (passed)."
