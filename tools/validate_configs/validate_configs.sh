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

set -e

run_test() {
	example=$1
	tmpdir="$(mktemp -d)"
	exampleFile=$(basename "$example")
	BLUEPRINT="${exampleFile%.yaml}-$(basename "${tmpdir##*.}")"

	echo "testing ${example} in ${tmpdir}"
	cp "${example}" "${tmpdir}/"
	cd "${tmpdir}"
	sed -i "s/blueprint_name: .*/blueprint_name: ${BLUEPRINT}/" "${exampleFile}" ||
		{
			echo "*** ERROR: could not set blueprint_name in ${example}"
			exit 1
		}

	PROJECT="invalid-project"

	sed -i "s/project_id: .*/project_id: ${PROJECT}/" "${exampleFile}" ||
		{
			echo "*** ERROR: could not set project_id in ${example}"
			exit 1
		}
	cd "${cwd}"
	./ghpc create -l IGNORE "${tmpdir}"/"${exampleFile}" >/dev/null ||
		{
			echo "*** ERROR: error creating blueprint with ghpc for ${exampleFile}"
			exit 1
		}
	mv "${BLUEPRINT}" "${tmpdir}"
	cd "${tmpdir}"/"${BLUEPRINT}" || {
		echo "*** ERROR: can't cd into the blueprint folder ${BLUEPRINT}"
		exit 1
	}
	for folder in ./*; do
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
			echo "neither packer nor terraform found in folder ${BLUEPRINT}/${folder}. Skipping."
		fi
		cd .. # back to blueprint folder
	done
	cd ..
	rm -rf "${BLUEPRINT}" || {
		echo "*** ERROR: could not remove blueprint folder from $(pwd)"
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

CONFIGS=$(find examples/ tools/validate_configs/test_configs/ -name "*.yaml" -type f)
cwd=$(pwd)
NPROCS=${NPROCS:-$(nproc)}
echo "Running tests in $NPROCS processes"
pids=()
for example in $CONFIGS; do
	JNUM=$(jobs | wc -l)
	# echo "$JNUM jobs running"
	if [ "$JNUM" -lt "$NPROCS" ]; then
		run_test "$example" &
		pids+=("$!")
	else
		# echo "Reached max number of parallel tests (${JNUM}). Waiting for one to finish."
		check_background
	fi
done
JNUM=$(jobs | wc -l)
while [ "$JNUM" -gt 0 ]; do
	check_background
	JNUM=$(jobs | wc -l)
done
echo "All configs have been validated successfully (passed)."
