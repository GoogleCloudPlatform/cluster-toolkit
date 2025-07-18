#!/bin/sh
# Copyright 2023 Google LLC
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

SCRIPT_COMPLETE_FILE="/run/startup_script_msg"

# Ensure we're in an interactive terminal and not root
if [ -t 1 ] && [ "$(id -u)" -ne 0 ]; then
	# Check if the file has contents otherwise skip
	if [ -s "$SCRIPT_COMPLETE_FILE" ]; then
		echo
		cat "$SCRIPT_COMPLETE_FILE"
		echo
	fi
fi
