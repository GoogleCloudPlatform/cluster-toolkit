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

# Default-deny safety wrapper blocking mutating gcluster subcommands (deploy|destroy, job submit|cancel).
script_dir="$(cd "$(dirname "$0")" && pwd)"
if [ "$(basename "$script_dir")" = ".live-shims" ]; then
	dir="$(cd "$script_dir/.." && pwd)"
	shim_dir="$script_dir"
else
	dir="$script_dir"
	shim_dir="$dir/.live-shims"
fi
audit_dir="${CTK_SHIM_AUDIT_DIR:-$shim_dir}"
real="$shim_dir/gcluster.real"
echo "gcluster $*" >>"$audit_dir/invocations.log"

help=0
bad_help=0
ver=0
sub1=""
sub2=""
sub3=""
skip=0
for a in "$@"; do
	if [ "$skip" = 1 ]; then
		skip=0
		continue
	fi
	case "$a" in
	--) break ;;
	--help | -h) help=1 ;;
	-v | --version) ver=1 ;;
	-s | -w | -c=* | -l=* | -p=* | --add-creator-label | --auto-approve | --await-job-completion | --download-dependencies | --enable-ml-diagnostics | --follow | --force | --gke-disable-parallel-containers | --gke-mtc-enabled | --json-output | --main-only | --no-color | --overwrite | --overwrite-deployment | --pathways | --pathways-deploy-sps | --pathways-headless | --robust | --show | --skip-gke-security-check | --skip-prereqs | --verbose | --*=*) ;;
	-f)
		if [ "$sub1" != "job" ] || [ "$sub2" != "logs" ]; then
			skip=1
		fi
		;;
	-B | -a | -b | -c | -d | -e | -i | -l | -n | -o | -p | -q | --artifacts | --backend-config | --base-image | --build-context | --cluster | --command | --compute-type | --cpu-affinity | --deployment-file | --dry-run-out | --env | --filter | --format | --gke-custom-templates-path | --gke-mtc-ramdisk-dir | --gke-namespace | --gke-nap-provisioning | --gke-nap-reservation | --gke-scheduler | --gke-ttl-after-finished | --grace-period | --image | --image-pull-secret | --location | --ml-diagnostics-image | --ml-diagnostics-telemetry-port | --mount | --name | --name-contains | --node-constraint | --num-nodes | --num-slices | --only | --out | --output | --output-dir | --parallelism | --pathways-colocated-python-sidecar-image | --pathways-elastic-slices | --pathways-gcs-location | --pathways-head-np | --pathways-max-slice-restarts | --pathways-proxy-args | --pathways-proxy-env | --pathways-proxy-server-image | --pathways-run-sps-workload | --pathways-server-args | --pathways-server-env | --pathways-server-image | --pathways-worker-args | --pathways-worker-env | --pathways-worker-image | --placement-policy | --platform | --priority | --project | --queue | --restart-on-exit-codes | --restarts | --service-account | --skip | --skip-validators | --status | --timeout | --topology | --validation-level | --vars)
		skip=1
		;;
	-*)
		if [ -z "$sub1" ]; then
			sub1="__blocked_opt__"
		elif [ -z "$sub2" ] && { [ "$sub1" = "cluster" ] || [ "$sub1" = "job" ]; }; then
			sub2="__blocked_opt__"
		elif [ -z "$sub3" ] && [ "$sub1" = "job" ] && [ "$sub2" = "config" ]; then
			sub3="__blocked_opt__"
		else
			bad_help=1
		fi
		;;
	*)
		if [ -z "$sub1" ]; then
			sub1="$a"
		elif [ -z "$sub2" ]; then
			sub2="$a"
		elif [ -z "$sub3" ]; then
			sub3="$a"
		fi
		;;
	esac
done

if [ "$bad_help" = 1 ] || [ "$sub1" = "__blocked_opt__" ] || [ "$sub2" = "__blocked_opt__" ] || [ "$sub3" = "__blocked_opt__" ]; then
	help=0
fi

ro=0
if [ "$#" -eq 0 ] || [ "$help" = 1 ]; then
	ro=1
elif [ "$ver" = 1 ] && [ -z "$sub1" ]; then
	ro=1
else
	case "$sub1" in
	deploy | destroy) ro=0 ;;
	help | version | completion | explain | expand | create) ro=1 ;;
	cluster)
		case "$sub2" in
		"" | describe | info | list | volume | help) ro=1 ;;
		esac
		;;
	job)
		case "$sub2" in
		"" | list | inspect | logs | gke-template-extract | help) ro=1 ;;
		config) [ "$sub3" = "show" ] && ro=1 ;;
		esac
		;;
	esac
fi

if [ "$ro" = 0 ]; then
	echo "gcluster $*" >>"$audit_dir/blocked.log"
	echo "Error: blocked execution of mutating command: gcluster $*" >&2
	exit 1
fi

if [ "$help" = 0 ] && [ "$#" -gt 0 ]; then
	if { [ "$sub1" = "cluster" ] && case "$sub2" in describe | info | list | volume) true ;; *) false ;; esac } ||
		{ [ "$sub1" = "job" ] && case "$sub2" in list | inspect | logs) true ;; *) false ;; esac } then
		echo "WARNING: offline evaluation sandbox: live cluster/job operations are disabled." >&2
		exit 0
	fi
fi

if [ ! -x "$real" ] && [ -f "$dir/go.mod" ]; then
	(
		cd "$dir" || exit 1
		rev="$(git rev-parse --short=12 HEAD 2>/dev/null || true)"
		cache="${CTK_BIN_CACHE_DIR:+${CTK_BIN_CACHE_DIR}/ctk-live-gcluster-$(id -u)-$rev}"
		if [ -n "$rev" ] && [ -n "$cache" ]; then
			(
				flock -x 9 2>/dev/null || true
				if [ ! -x "$cache" ]; then
					go build -o "$cache.tmp.$$" . >/dev/null 2>&1 && chmod 755 "$cache.tmp.$$" && mv -f "$cache.tmp.$$" "$cache"
				fi
			) 9>"$cache.lock"
			[ -x "$cache" ] && cp -f "$cache" "$real.tmp.$$" && chmod 755 "$real.tmp.$$" && mv -f "$real.tmp.$$" "$real"
		else
			go build -o "$real.tmp.$$" . >/dev/null 2>&1 && chmod 755 "$real.tmp.$$" && mv -f "$real.tmp.$$" "$real"
		fi
		find "$dir/modules" "$dir/community/modules" -name '*.tf' -exec touch -a -t 200001010000 {} + 2>/dev/null || true
	) || true
fi

if [ ! -x "$real" ]; then
	echo "Error: gcluster binary not built" >&2
	exit 1
fi
if [ "$help" = 1 ]; then
	exec "$real" ${sub1:+"$sub1"} ${sub2:+"$sub2"} ${sub3:+"$sub3"} --help
fi
exec "$real" "$@"
