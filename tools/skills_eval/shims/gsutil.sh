#!/usr/bin/env bash
# Copyright 2026 Google LLC
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

shim_dir="$(cd "$(dirname "$0")" && pwd)"
audit_dir="${CTK_SHIM_AUDIT_DIR:-$shim_dir}"

help=0
ver=0
skip=0
sub=""
for a in "$@"; do
	if [ "$skip" = 1 ]; then
		skip=0
		continue
	fi
	case "$a" in
	--) break ;;
	--help | "-?") [ -z "$sub" ] && help=1 ;;
	-v | --version) [ -z "$sub" ] && ver=1 ;;
	-h | -i | -o | -u | --impersonate-service-account | --trace-token | --perf-trace-token) [ -z "$sub" ] && skip=1 ;;
	-d | -D | -DD | -m | -q | -qm | -mq | --debug | --detailedDebug | --multithreaded | --quiet | --testexceptiontraces | --*=*) ;;
	-*) [ -z "$sub" ] && sub="__blocked_opt__" ;;
	*)
		if [ -z "$sub" ]; then
			sub="$a"
		fi
		;;
	esac
done

ro=0
if [ "$#" -eq 0 ] || { [ "$help" = 1 ] && [ -z "$sub" ]; }; then
	ro=1
elif [ "$ver" = 1 ] && [ -z "$sub" ]; then
	ro=1
else
	case "$sub" in
	help | version | ls | stat | du | cat) ro=1 ;;
	esac
fi

if [ "$ro" = 0 ]; then
	echo "gsutil $*" >>"$audit_dir/blocked.log"
	echo "Error: blocked execution of mutating command: gsutil $*" >&2
	exit 1
fi

echo "gsutil $*" >>"$audit_dir/invocations.log"
if [ "$sub" = "version" ] || { [ "$ver" = 1 ] && [ -z "$sub" ]; }; then
	echo "gsutil version: offline-shim"
	exit 0
fi
echo "CommandException: offline evaluation sandbox: live GCS calls are disabled" >&2
exit 0
