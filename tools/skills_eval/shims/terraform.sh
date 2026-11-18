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
case "${1:-}" in
version | -version | --version | -help | --help | -h | "")
	exec "$shim_dir/terraform.real" "$@"
	;;
esac
echo "terraform $*" >>"$audit_dir/blocked.log"
echo "Error: refusing 'terraform $1' in this environment." >&2
exit 1
