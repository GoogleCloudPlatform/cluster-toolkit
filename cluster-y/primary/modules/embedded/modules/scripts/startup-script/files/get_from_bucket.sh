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

# Given a url and filename, download an object to the vardir. When the installed
# version of gcloud is >=402.0.0 (Sept. 2022), then gcloud storage is used to
# fetch from the bucket. Otherwise gsutil is used. Note, the service account for
# the instance must be properly configured with a role having authorization to
# get objects from the bucket.
#
# This function is intended for single file downloads and no attempt is made to
# verify the checksum other than the default behavior of gcloud or gsutil.
#
# This function has no other platform dependencies other than gcloud / gsutil.

# This code originated from: https://github.com/terraform-google-modules/terraform-google-startup-scripts?ref=v1.0.0
stdlib::get_from_bucket() {
	local OPTIND opt url fname dir="${VARDIR:-/var/lib/startup}"
	while getopts ":u:f:d:" opt; do
		case "${opt}" in
		u) url="${OPTARG}" ;;
		f) fname="${OPTARG}" ;;
		d) dir="${OPTARG}" ;;
		:)
			stdlib::mandatory_argument -n stdlib::get_from_bucket -f "$OPTARG"
			return "${E_MISSING_MANDATORY_ARG}"
			;;
		*)
			stdlib::error 'Usage: stdlib::get_from_bucket -u <url> -f <file name> -d <directory>'
			stdlib::info 'For example: stdlib::get_from_bucket -u gs://mybucket/foo.tgz -d /var/tmp'
			return "${E_UNKNOWN_ARG}"
			;;
		esac
	done
	# Trivially compute the filename from the URL if unspecified.
	if [[ -z ${fname} ]]; then
		fname=${url##*/}
		stdlib::debug "Computed filename='${fname}' given URL."
	fi
	[[ -d ${dir} ]] || mkdir "${dir}"
	local attempt=0
	local max_retries=7
	# store gcs command as array and then split when called by stdlib::cmd
	if stdlib::cmd gcloud help storage cp &>/dev/null; then
		gcs_command=(gcloud storage cp --no-user-output-enabled)
	else
		gcs_command=(gsutil -q cp)
	fi
	while [[ $attempt -le $max_retries ]]; do
		if [[ $attempt -gt 0 ]]; then
			local wait=$((2 ** attempt))
			stdlib::error "Retry attempt ${attempt} of ${max_retries} with exponential backoff: ${wait} seconds."
			sleep $wait
		fi
		if stdlib::cmd "${gcs_command[@]}" "${url}" "${dir}/${fname}"; then
			break
		else
			stdlib::error "${gcs_command[*]} reported non-zero exit code fetching ${url}."
			((attempt++))
		fi
	done
}
