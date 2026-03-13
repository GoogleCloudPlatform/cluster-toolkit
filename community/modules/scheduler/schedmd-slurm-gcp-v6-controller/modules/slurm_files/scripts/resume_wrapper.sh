#!/bin/bash
#
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

set -e -o pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
PYTHON_SCRIPT="${SCRIPT_DIR}/resume.py"

# Capture all arguments passed by Slurm (the nodelist).
ALL_ARGS=("$@")

# This array will hold extra argument for resume.py, like the resume data file.
UNIQUE_RESUME_FILE=""

# Handle SLURM_RESUME_FILE if provided
if [ -n "${SLURM_RESUME_FILE-}" ] && [ -f "$SLURM_RESUME_FILE" ]; then
	SAFE_DIR="/tmp/slurm_resume_data"
	mkdir -p "$SAFE_DIR"

	UNIQUE_RESUME_FILE="${SAFE_DIR}/resumedata.$$.json"
	cp "$SLURM_RESUME_FILE" "$UNIQUE_RESUME_FILE"
fi

SLURM_RESUME_FILE="${UNIQUE_RESUME_FILE}"
setsid "${PYTHON_SCRIPT}" "${ALL_ARGS[@]}" &

exit 0
