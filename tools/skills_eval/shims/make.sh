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
wt_d="$(cd "$shim_dir/.." && pwd)"
audit_dir="${CTK_SHIM_AUDIT_DIR:-$shim_dir}"
real_gc="$shim_dir/gcluster.real"
real_mk="$shim_dir/make.real"

custom=0
force=0
want_help=0
want_ver=0
skip=0
numskip=0
other=0
for a in "$@"; do
	if [ "$skip" = 1 ]; then
		skip=0
		continue
	fi
	if [ "$numskip" = 1 ]; then
		numskip=0
		case "$a" in
		[0-9]* | .[0-9]*) continue ;;
		esac
	fi
	case "$a" in
	-C | --directory | -f | --file | --makefile | -E | --eval)
		custom=1
		skip=1
		;;
	--directory=* | --file=* | --makefile=* | --eval=* | -C* | -f* | -E* | -[!-]*[CfE]*) custom=1 ;;
	-I | --include-dir | -W | --what-if | -o | --old-file) skip=1 ;;
	-j | --jobs | -l | --load-average | --max-load) numskip=1 ;;
	-B | --always-make) force=1 ;;
	-h | --help) want_help=1 ;;
	--version) want_ver=1 ;;
	-v)
		if [ "$#" -eq 1 ]; then
			want_ver=1
		else
			other=1
		fi
		;;
	-[!-]*)
		case "$a" in
		*B*) force=1 ;;
		esac
		case "$a" in
		*[jl]) numskip=1 ;;
		esac
		;;
	-* | *=*) ;;
	gcluster | ghpc | all | build) ;;
	*) other=1 ;;
	esac
done

if [ "$custom" = 0 ] && [ "$other" = 0 ]; then
	if [ "$want_help" = 1 ] && [ -x "$real_mk" ]; then
		exec "$real_mk" --help
	fi
	if [ "$want_ver" = 1 ] && [ -x "$real_mk" ]; then
		exec "$real_mk" --version
	fi
	if [ "$force" = 0 ] && [ -x "$real_gc" ]; then
		echo "gcluster is already built and up to date."
		exit 0
	fi
	if (cd "$wt_d" && go build -o "$real_gc.tmp.$$" . && chmod 755 "$real_gc.tmp.$$" && mv -f "$real_gc.tmp.$$" "$real_gc"); then
		find "$wt_d/modules" "$wt_d/community/modules" -name '*.tf' -exec touch -a -t 200001010000 {} + 2>/dev/null || true
		exit 0
	fi
	exit 1
fi

echo "make $*" >>"$audit_dir/blocked.log"
echo "Error: blocked execution of non-build target: make $*" >&2
exit 1
