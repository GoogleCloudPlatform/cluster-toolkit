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
pos=()
for a in "$@"; do
	if [ "$skip" = 1 ]; then
		skip=0
		continue
	fi
	case "$a" in
	--) break ;;
	--help | -h) help=1 ;;
	-v | --version) ver=1 ;;
	--*=*) ;;
	--project | --account | --configuration | --format | --filter | --flatten | --limit | --page-size | --sort-by | --verbosity | --zone | --region | --location | --cluster | --node-pool | --machine-type | --network | --subnetwork | --impersonate-service-account | --billing-project | --flags-file | --command | --access-token-file | --trace-token)
		skip=1
		;;
	-*) ;;
	*) pos+=("$a") ;;
	esac
done

if [ "${pos[0]:-}" = "alpha" ] || [ "${pos[0]:-}" = "beta" ]; then
	pos=("${pos[@]:1}")
fi
n="${#pos[@]}"
f="${pos[0]:-}"
s="${pos[1]:-}"
ro=0

if [ "$#" -eq 0 ] || [ "$help" = 1 ]; then
	ro=1
elif [ "$ver" = 1 ] && [ "$n" -eq 0 ]; then
	ro=1
elif [ "$f" = "version" ] || [ "$f" = "info" ]; then
	[ "$n" -eq 1 ] && ro=1
elif [ "$f" = "help" ]; then
	ro=1
elif [ "$f" = "auth" ]; then
	[ "$n" -eq 2 ] && [ "$s" = "list" ] && ro=1
elif [ "$f" = "config" ]; then
	case "$s" in
	list) [ "$n" -le 3 ] && ro=1 ;;
	get-value | get) [ "$n" -eq 3 ] && ro=1 ;;
	esac
elif [ "$n" -ge 2 ]; then
	bad=0
	verb_idx=-1
	for i in "${!pos[@]}"; do
		[ "$i" -eq 0 ] && continue
		tok="${pos[$i]}"
		case "$tok" in
		delete | delete-* | destroy | destroy-* | create | create-* | update | update-* | patch | patch-* | set | set-* | unset | unset-* | reset | reset-* | add | add-* | remove | remove-* | rm | cp | mv | rsync | upload | upload-* | deploy | deploy-* | submit | submit-* | cancel | cancel-* | run | exec | exec-* | ssh | scp | start | start-* | stop | stop-* | restart | restart-* | resize | resize-disk | import | import-* | export | export-* | restore | restore-* | enable | enable-* | disable | disable-* | attach | attach-* | detach | detach-* | undelete | undelete-* | move | move-* | copy | copy-* | rollback | rollback-* | upgrade | upgrade-* | drain | drain-* | cordon | cordon-* | login | activate | activate-* | revoke | revoke-* | print-* | sign-* | get-credentials | install | insert | publish | access | decrypt)
			if [ "$verb_idx" -lt 0 ]; then
				bad=1
				break
			fi
			;;
		list | describe)
			if [ "$verb_idx" -lt 0 ]; then
				verb_idx="$i"
			fi
			;;
		esac
	done
	if [ "$bad" = 0 ] && [ "$verb_idx" -ge 1 ]; then
		ro=1
	fi
fi

if [ "$ro" = 0 ]; then
	echo "gcloud $*" >>"$audit_dir/blocked.log"
	echo "Error: blocked execution of mutating or unpermitted command: gcloud $*" >&2
	exit 1
fi

echo "gcloud $*" >>"$audit_dir/invocations.log"
if [ "$f" = "version" ] || { [ "$ver" = 1 ] && [ "$n" -eq 0 ]; }; then
	echo "Google Cloud SDK (offline evaluation shim)"
	exit 0
fi
echo "WARNING: offline evaluation sandbox: live gcloud API calls are disabled." >&2
exit 0
