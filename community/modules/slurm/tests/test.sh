#!/bin/bash
# Copyright 2023 Google LLC
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

set -e

validator_to_skip="test_project_exists,test_apis_enabled,test_region_exists,test_zone_exists,test_zone_in_region"
tmpdir=$(mktemp -d)
tstdir="community/modules/slurm/tests"

./ghpc create -l ERROR \
	--skip-validators="${validator_to_skip}" \
	"${tstdir}/bp.yaml" -o "${tmpdir}" >/dev/null

depldir="${tmpdir}/test/primary"
terraform -chdir="${depldir}" init >/dev/null
terraform -chdir="${depldir}" validate >/dev/null
terraform -chdir="${depldir}" apply --auto-approve >/dev/null

for expectations in "$tstdir"/expectations/*; do
	vn=$(basename "$expectations")
	vn="${vn%.*}" # trim extension
	output="${tmpdir}/out_${vn}"
	terraform -chdir="${depldir}" output "${vn}" >"${output}"
	diff --color "${output}" "${expectations}" || {
		echo "FAIL: ${output} does not match ${expectations}"
		exit 1 # TODO: don't terminate after first failure
	}
	echo "PASS: ${vn}"
done

rm -rf "${tmpdir}"
