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

# This code contains minor changes from the original in: https://github.com/terraform-google-modules/terraform-google-startup-scripts?ref=v1.0.0

# Standard library of functions useful for startup scripts.

# These are outside init_global_vars so logging functions work with the most
# basic case of `source startup-script-stdlib.sh`
readonly SYSLOG_DEBUG_PRIORITY="${SYSLOG_DEBUG_PRIORITY:-syslog.debug}"
readonly SYSLOG_INFO_PRIORITY="${SYSLOG_INFO_PRIORITY:-syslog.info}"
readonly SYSLOG_ERROR_PRIORITY="${SYSLOG_ERROR_PRIORITY:-syslog.error}"
# Global counter of how many times stdlib::init() has been called.
STARTUP_SCRIPT_STDLIB_INITIALIZED=0

# Error codes
readonly E_RUN_OR_DIE=5
readonly E_MISSING_MANDATORY_ARG=9
readonly E_UNKNOWN_ARG=10

SCRIPT_COMPLETE_FILE="/run/startup_script_msg"
SUCCESS_MESSAGE="* NOTICE **: The HPC Toolkit startup scripts have finished running successfully."
readonly SUCCESS_MESSAGE
ERROR_MESSAGE="** ERROR **: The HPC Toolkit startup scripts have finished running, but produced an error."
readonly ERROR_MESSAGE
WARNING_MESSAGE="** WARNING **: The HPC Toolkit startup scripts are currently running."
readonly WARNING_MESSAGE

stdlib::debug() {
	[[ -z ${DEBUG:-} ]] && return 0
	local ds msg
	msg="$*"
	logger -p "${SYSLOG_DEBUG_PRIORITY}" -t "${PROG}[$$]" -- "${msg}"
	[[ -n ${QUIET:-} ]] && return 0
	ds="$(date +"${DATE_FMT}") "
	echo -e "${BLUE}${ds}Debug [$$]: ${msg}${NC}" >&2
}

stdlib::info() {
	local ds msg
	msg="$*"
	logger -p "${SYSLOG_INFO_PRIORITY}" -t "${PROG}[$$]" -- "${msg}"
	[[ -n ${QUIET:-} ]] && return 0
	ds="$(date +"${DATE_FMT}") "
	echo -e "${GREEN}${ds}Info [$$]: ${msg}${NC}" >&2
}

stdlib::error() {
	local ds msg
	msg="$*"
	ds="$(date +"${DATE_FMT}") "
	logger -p "${SYSLOG_ERROR_PRIORITY}" -t "${PROG}[$$]" -- "${msg}"
	echo -e "${RED}${ds}Error [$$]: ${msg}${NC}" >&2
}

stdlib::announce_runners_start() {
	if [ -z "$recursive_proc" ]; then
		wall -n "$WARNING_MESSAGE"
		echo "$WARNING_MESSAGE" >"$SCRIPT_COMPLETE_FILE"
	fi
	export recursive_proc=$((${recursive_proc:=0} + 1))
}

stdlib::announce_runners_end() {
	exit_code=$1
	export recursive_proc=$((${recursive_proc:=0} - 1))
	if [ "$recursive_proc" -le "0" ]; then
		if [ "$exit_code" -ne "0" ]; then
			wall -n "$ERROR_MESSAGE"
			echo "$ERROR_MESSAGE" >"$SCRIPT_COMPLETE_FILE"
		else
			wall -n "$SUCCESS_MESSAGE"
			echo -n "" >"$SCRIPT_COMPLETE_FILE"
		fi
	fi
}

# The main initialization function of this library.  This should be kept to the
# minimum amount of work required for all functions to operate cleanly.
stdlib::init() {
	if [[ ${STARTUP_SCRIPT_STDLIB_INITIALIZED} -gt 0 ]]; then
		stdlib::info 'stdlib::init()'" already initialized, no action taken."
		return 0
	fi
	((STARTUP_SCRIPT_STDLIB_INITIALIZED++)) || true
	stdlib::init_global_vars
	stdlib::init_directories
	stdlib::debug "stdlib::init(): startup-script-stdlib.sh initialized and ready"
}

# Initialize global variables.
stdlib::init_global_vars() {
	# The program name, used for logging.
	readonly PROG="${PROG:-startup-script-stdlib}"
	# Date format used for stderr logging.  Passed to date + command.
	readonly DATE_FMT="${DATE_FMT:-"%a %b %d %H:%M:%S %z %Y"}"
	# var directory
	readonly VARDIR="${VARDIR:-/var/lib/startup}"
	# Override this with file://localhost/tmp/foo/bar in spec test context
	readonly METADATA_BASE="${METADATA_BASE:-http://metadata.google.internal}"

	# Color variables
	if [[ -n ${COLOR:-} ]]; then
		readonly NC='\033[0m'       # no color
		readonly RED='\033[0;31m'   # error
		readonly GREEN='\033[0;32m' # info
		readonly BLUE='\033[0;34m'  # debug
	else
		readonly NC=''
		readonly RED=''
		readonly GREEN=''
		readonly BLUE=''
	fi

	return 0
}

stdlib::init_directories() {
	if ! [[ -e ${VARDIR} ]]; then
		install -d -m 0755 -o 0 -g 0 "${VARDIR}"
	fi
}

##
# Get a metadata key.  When used without -o, this function is guaranteed to
# produce no output on STDOUT other than the retrieved value.  This is intended
# to support the use case of
# FOO="$(stdlib::metadata_get -k instance/attributes/foo)"
#
# If the requested key does not exist, the error code will be 22 and zero bytes
# written to STDOUT.
stdlib::metadata_get() {
	local OPTIND opt key outfile
	local metadata="${METADATA_BASE%/}/computeMetadata/v1"
	local exit_code
	while getopts ":k:o:" opt; do
		case "${opt}" in
		k) key="${OPTARG}" ;;
		o) outfile="${OPTARG}" ;;
		:)
			stdlib::error "Invalid option: -${OPTARG} requires an argument"
			stdlib::metadata_get_usage
			return "${E_MISSING_MANDATORY_ARG}"
			;;
		*)
			stdlib::error "Unknown option: -${opt}"
			stdlib::metadata_get_usage
			return "${E_UNKNOWN_ARG}"
			;;
		esac
	done
	local url="${metadata}/${key#/}"

	stdlib::debug "Getting metadata resource url=${url}"
	if [[ -z ${outfile:-} ]]; then
		curl --location --silent --connect-timeout 1 --fail \
			-H 'Metadata-Flavor: Google' "$url" 2>/dev/null
		exit_code=$?
	else
		stdlib::cmd curl --location \
			--silent \
			--connect-timeout 1 \
			--fail \
			--output "${outfile}" \
			-H 'Metadata-Flavor: Google' \
			"$url"
		exit_code=$?
	fi
	case "${exit_code}" in
	22 | 37)
		stdlib::debug "curl exit_code=${exit_code} for url=${url}" \
			"(Does not exist)"
		;;
	esac
	return "${exit_code}"
}

stdlib::metadata_get_usage() {
	stdlib::info 'Usage: stdlib::metadata_get -k <key>'
	stdlib::info 'For example: stdlib::metadata_get -k instance/attributes/startup-config'
}

# Load configuration values in the spirit of /etc/sysconfig defaults, but from
# metadata instead of the filesystem.
stdlib::load_config_values() {
	local config_file
	local key="instance/attributes/startup-script-config"
	# shellcheck disable=SC2119
	config_file="$(stdlib::mktemp)"
	stdlib::metadata_get -k "${key}" -o "${config_file}"
	local status=$?
	case "$status" in
	0)
		stdlib::debug "SUCCESS: Configuration data sourced from $key"
		;;
	22 | 37)
		stdlib::debug "no configuration data loaded from $key"
		;;
	*)
		stdlib::error "metadata_get -k $key returned unknown status=${status}"
		;;
	esac
	# shellcheck source=/dev/null
	source "${config_file}"
}

# Run a command logging the entry and exit.  Intended for system level commands
# and operational debugging.  Not intended for use with redirection.  This is
# not named run() because bats uses a run() function.
stdlib::cmd() {
	local exit_code argv=("$@")
	stdlib::debug "BEGIN: stdlib::cmd() command=[${argv[*]}]"
	"${argv[@]}"
	exit_code=$?
	stdlib::debug "END: stdlib::cmd() command=[${argv[*]}] exit_code=${exit_code}"
	return $exit_code
}

# Run a command successfully or exit the program with an error.
stdlib::run_or_die() {
	if ! stdlib::cmd "$@"; then
		stdlib::error "stdlib::run_or_die(): exiting with exit code ${E_RUN_OR_DIE}."
		exit "${E_RUN_OR_DIE}"
	fi
}

# Intended to take advantage of automatic cleanup of startup script library
# temporary files without exporting a modified TMPDIR to child processes, which
# would cause the children to have their TMPDIR deleted out from under them.
# shellcheck disable=SC2120
stdlib::mktemp() {
	TMPDIR="${DELETE_AT_EXIT:-${TMPDIR}}" mktemp "$@"
}

# Return a nice error message if a mandatory argument is missing.
stdlib::mandatory_argument() {
	local OPTIND opt name flag
	while getopts ":n:f:" opt; do
		case "$opt" in
		n) name="${OPTARG}" ;;
		f) flag="${OPTARG}" ;;
		:)
			stdlib::error "Invalid argument: -${OPTARG} requires an argument to stdlib::mandatory_argument()"
			return "${E_MISSING_MANDATORY_ARG}"
			;;
		*)
			stdlib::error "Unknown argument: -${OPTARG}"
			stdlib::info "Usage: stdlib::mandatory_argument -n <name> -f <flag>"
			return "${E_UNKNOWN_ARG}"
			;;
		esac
	done
	stdlib::error "Invalid argument: -${flag} requires an argument to ${name}()."
}
