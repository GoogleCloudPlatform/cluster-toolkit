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
bad_help=0
sub1=""
sub2=""
skip=0
for a in "$@"; do
	if [ "$skip" = 1 ]; then
		skip=0
		continue
	fi
	case "$a" in
	--) break ;;
	--help | -h) help=1 ;;
	--disable-compression | --insecure-skip-tls-verify | --match-server-version | --warnings-as-errors | --*=* | -n?* | -s?* | -v?*) ;;
	-n | --namespace | -s | --server | --context | --cluster | --user | --username | --password | --kubeconfig | --kuberc | --as | --as-uid | --as-group | --as-user-extra | --token | --client-certificate | --client-key | --certificate-authority | --tls-server-name | --request-timeout | --cache-dir | --profile | --profile-output | --log-flush-frequency | --vmodule | -v | --v | -o | --output | -l | --selector | --field-selector | -f | --filename | -k | --kustomize | -c | --container | -p | --patch | --patch-file | --type)
		skip=1
		;;
	-*)
		if [ -z "$sub1" ]; then
			sub1="__blocked_opt__"
		elif [ -z "$sub2" ] && { [ "$sub1" = "auth" ] || [ "$sub1" = "rollout" ] || [ "$sub1" = "config" ]; }; then
			sub2="__blocked_opt__"
		else
			bad_help=1
		fi
		;;
	*)
		if [ -z "$sub1" ]; then
			sub1="$a"
		elif [ -z "$sub2" ]; then
			sub2="$a"
		fi
		;;
	esac
done

ro=0
if [ "$#" -eq 0 ] || { [ "$help" = 1 ] && [ "$bad_help" = 0 ] && [ "$sub1" != "__blocked_opt__" ] && [ "$sub2" != "__blocked_opt__" ]; }; then
	ro=1
else
	case "$sub1" in
	get | describe | logs | top | events | explain | api-resources | api-versions | version | cluster-info | help) ro=1 ;;
	auth)
		case "$sub2" in
		can-i | whoami) ro=1 ;;
		esac
		;;
	rollout)
		case "$sub2" in
		status | history) ro=1 ;;
		esac
		;;
	config)
		case "$sub2" in
		view | get-contexts | current-context | get-clusters | get-users) ro=1 ;;
		esac
		;;
	esac
fi

if [ "$ro" = 0 ]; then
	echo "kubectl $*" >>"$audit_dir/blocked.log"
	echo "Error from server (Forbidden): mutating command blocked: kubectl $*" >&2
	exit 1
fi

echo "kubectl $*" >>"$audit_dir/invocations.log"
# Intentional exit 0: delivers connection-refused stderr without triggering CLI tool-error retry loops.
echo "Unable to connect to the server: dial tcp 127.0.0.1:8080: connect: connection refused" >&2
exit 0
