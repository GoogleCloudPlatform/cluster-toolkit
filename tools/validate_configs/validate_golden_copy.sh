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

set -e

run_test() {
	bp=$1
	gc=$2
	tmpdir="$(mktemp -d)"
	bpFile=$(basename "$bp")
	DEPLOYMENT="golden_copy_deployment"
	PROJECT="invalid-project"
	VALIDATORS_TO_SKIP="test_project_exists,test_apis_enabled,test_region_exists,test_zone_exists,test_zone_in_region"
	GHPC_PATH="${cwd}/ghpc"
	# Cover the three possible starting sequences for local sources: ./ ../ /
	LOCAL_SOURCE_PATTERN='source:\s\+\(\./\|\.\./\|/\)'

	ls "${gc}" >/dev/null 2>&1 || {
		echo "*** ERROR: ${gc} folder not found"
		exit 1
	}
	untracked=$(find "${gc}" -type f -print | git check-ignore --stdin || true)
	if [[ -n "${untracked}" ]]; then
		echo "*** ERROR: ${gc} folder contains untracked files:"
		echo "${untracked}"
		exit 1
	fi

	echo "testing ${bp} in ${tmpdir} against ${gc}"
	cp "${bp}" "${tmpdir}/"

	# Only run from the repo directory if there are local modules, otherwise
	# run the test from the test directory using the installed ghpc binary.
	if grep -q "${LOCAL_SOURCE_PATTERN}" "${cwd}/${bp}"; then
		cd "${cwd}"
	else
		cd "${tmpdir}"
	fi
	${GHPC_PATH} create -l ERROR \
		--skip-validators="${VALIDATORS_TO_SKIP}" \
		--vars="project_id=${PROJECT},deployment_name=${DEPLOYMENT}" \
		"${tmpdir}"/"${bpFile}" >/dev/null ||
		{
			echo "*** ERROR: error creating deployment with ghpc for ${bpFile}"
			exit 1
		}
	if grep -q "${LOCAL_SOURCE_PATTERN}" "${cwd}/${bp}"; then
		mv "${DEPLOYMENT}" "${tmpdir}"
	fi
	cd "${tmpdir}"/"${DEPLOYMENT}" || {
		echo "*** ERROR: can't cd into the deployment folder ${DEPLOYMENT}"
		exit 1
	}

	# Sanitize deployment folder
	rm .gitignore
	for folder in ./*; do
		rm -rf "${folder}/modules"
	done
	find . -name "README.md" -exec rm {} \;
	# Add license headers to yaml files
	addlicense -c "Google LLC" -l apache .

	# Compare the deployment folder with the golden copy
	diff --recursive --exclude="previous_deployment_groups" \
		"$(pwd)" "${cwd}/${gc}" || {
		echo "*** ERROR: ${tmpdir}/${DEPLOYMENT} does not match ${gc}"
		exit 1
	}

	rm -rf "${DEPLOYMENT}" || {
		echo "*** ERROR: could not remove deployment folder from $(pwd)"
		exit 1
	}
	cd "${cwd}"
	rm -r "${tmpdir}"
}

cwd=$(pwd)
gcs="tools/validate_configs/golden_copies"
ls ${gcs} >/dev/null 2>&1 || {
	echo "*** ERROR: ${gcs} folder not found try running from the root of the repo"
	exit 1
}
# Tests:
run_test "tools/validate_configs/test_configs/igc_pkr_test.yaml" "${gcs}/packer_igc"
run_test "tools/validate_configs/test_configs/igc_tf_test.yaml" "${gcs}/terraform_igc"
