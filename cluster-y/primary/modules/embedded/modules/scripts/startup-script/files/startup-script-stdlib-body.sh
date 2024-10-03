#! /bin/bash
# Copyright 2018 Google LLC
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

# This code contains minor changes from the original: https://github.com/terraform-google-modules/terraform-google-startup-scripts?ref=v1.0.0

stdlib::main() {
	DELETE_AT_EXIT="$(mktemp -d)"
	readonly DELETE_AT_EXIT

	# Initialize state required by other functions, e.g. debug()
	stdlib::init
	stdlib::debug "Loaded startup-script-stdlib as an executable."

	stdlib::load_config_values

	stdlib::load_runners
}

# if script is being executed and not sourced.
if [[ ${BASH_SOURCE[0]} == "${0}" ]]; then
	stdlib::finish() {
		[[ -d ${DELETE_AT_EXIT:-} ]] && rm -rf "${DELETE_AT_EXIT}"
	}
	trap stdlib::finish EXIT

	stdlib::main "$@"
fi
