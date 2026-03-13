#!/bin/bash
# Copyright 2026 "Google LLC"
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

# This creates a file named "nvidia+pytorch+24.09-py3.sqsh", which
# uses ~18 GB of disk space. This should be run on a filesystem that
# can be seen by all worker nodes

# Fix for non-interactive shells where XDG_RUNTIME_DIR is not set
if [ -z "$XDG_RUNTIME_DIR" ]; then
	# Try creating a user-specific directory in /run (often fails for non-root)
	XDG_RUNTIME_DIR="/run/user/$(id -u)"
	export XDG_RUNTIME_DIR

	# Check if we can actually use it
	if [ ! -d "$XDG_RUNTIME_DIR" ]; then
		# Fallback to a guaranteed writable location in /tmp
		XDG_RUNTIME_DIR="/tmp/enroot-runtime-$(id -u)"
		export XDG_RUNTIME_DIR
		mkdir -p "$XDG_RUNTIME_DIR"
		chmod 700 "$XDG_RUNTIME_DIR"
	fi
fi

# Import the pytorch container
enroot import "docker://nvcr.io#nvidia/pytorch:24.09-py3"
