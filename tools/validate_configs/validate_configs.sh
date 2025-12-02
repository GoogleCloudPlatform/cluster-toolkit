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
	DEPLOYMENT=$(echo "${exampleFile%.yaml}-$(basename "${tmpdir##*.}")" | sed -e 's/\(.*\)/\L\1/')
	PROJECT="invalid-project"
	VALIDATORS_TO_SKIP="test_project_exists,test_apis_enabled,test_region_exists,test_zone_exists,test_zone_in_region"
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
	${GHPC_PATH} create "${BP_PATH}" -l ERROR \
		--skip-validators="${VALIDATORS_TO_SKIP}" "${deployment_args[@]}" \
		--vars="project_id=${PROJECT},deployment_name=${DEPLOYMENT}" >/dev/null ||
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
		pkrdirs=()
		while IFS= read -r -d $'\n'; do
			pkrdirs+=("$REPLY")
		done < <(find . -name "*.pkr.hcl" -printf '%h\n' | sort -u)
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
	# "wait -n" was introduced in bash 4.3; support CentOS 7: 4.2 and MacOS: 3.2!
	if [[ "${BASH_VERSINFO[0]}" -ge 5 || "${BASH_VERSINFO[0]}" -eq 4 && "${BASH_VERSINFO[1]}" -ge 3 ]]; then
		if ! wait -n; then
			wait
			echo "*** ERROR: a test failed. Exiting with status 1."
			exit 1

		fi
	else
		failed=0
		for pid in "${pids[@]}"; do
			if ! wait "$pid"; then
				failed=1
			fi
		done
		pids=()

		if [[ $failed -eq 1 ]]; then
			echo "*** ERROR: a test failed. Exiting with status 1."
			exit 1
		fi
	fi
}

CONFIGS=$(find examples/ community/examples/ tools/validate_configs/test_configs/ docs/tutorials/ docs/videos/build-your-own-blueprint/ -name "*.yaml" -type f -not -path 'examples/machine-learning/a3-megagpu-8g/*' -not -path 'examples/machine-learning/a3-ultragpu-8g/*' -not -path 'examples/machine-learning/build-service-images/*' -not -path 'examples/gke-a3-ultragpu/*' -not -path 'examples/hypercompute_clusters/*' -not -path 'examples/gke-consumption-options/*' -not -path 'examples/gke-a4/*' -not -path 'examples/gke-a3-megagpu/*' -not -path 'examples/machine-learning/a4-highgpu-8g/*' -not -path 'examples/machine-learning/a4x-highgpu-4g/*' -not -path 'community/examples/gke-tpu-v6/*' -not -path 'community/examples/xpk-n2-filestore/*' -not -path 'examples/gke-a4x/*' -not -path 'examples/science/af3-slurm/*' -not -path 'examples/gke-h4d/*' -not -path 'community/examples/hpc-slinky/*' -not -path 'examples/gke-g4/*' -not -path 'community/examples/slurm-gke/*' -not -path 'examples/hpc-slurm-h4d/*' -not -path 'examples/machine-learning/a3-highgpu-8g/*')
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

	JNUM=$(jobs | wc -l)
	# echo "$JNUM jobs running"
	if [ "$JNUM" -ge "$NPROCS" ]; then
		check_background
	fi
	run_test "$example" &
	pids+=("$!")
done
JNUM=$(jobs | wc -l)
while [ "$JNUM" -gt 0 ]; do
	check_background
	JNUM=$(jobs | wc -l)
done

run_test "examples/science/af3-slurm/af3-slurm.yaml" "examples/science/af3-slurm/af3-slurm-deployment.yaml"

echo "All configs have been validated successfully (passed)."
