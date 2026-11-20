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
help_ver=0
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
	-C | --directory | -f | --file | --makefile)
		custom=1
		skip=1
		;;
	--directory=* | --file=* | --makefile=* | -C* | -f*) custom=1 ;;
	-I | --include-dir | -W | --what-if | -o | --old-file) skip=1 ;;
	-j | --jobs | -l | --load-average | --max-load) numskip=1 ;;
	-B | --always-make) force=1 ;;
	-h | --help | -v | --version) help_ver=1 ;;
	-* | *=*) ;;
	gcluster | ghpc | all | build) ;;
	*) other=1 ;;
	esac
done

if [ "$custom" = 0 ] && [ "$other" = 0 ]; then
	if [ "$help_ver" = 1 ] && [ -x "$real_mk" ]; then
		exec "$real_mk" "$@"
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
