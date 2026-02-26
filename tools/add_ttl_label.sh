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

set -euo pipefail

# tools/add_ttl_label.sh
# Scoped script to add a TTL label ONLY under the top-level 'vars:' block.

if [ "$#" -ne 1 ]; then
	echo "Usage: $0 <blueprint-file>"
	exit 1
fi

BLUEPRINT_FILE=$1

# Check if file exists
if [ ! -f "$BLUEPRINT_FILE" ]; then
	echo "ERROR: Blueprint file not found: $BLUEPRINT_FILE" >&2
	exit 1
fi

# Calculate TTL (Current UTC + 4 hours)
TTL_LABEL=$(date -u -d '+4 hours' +'%Y-%m-%d_%H-%M')

# 1. Identify the boundaries of the 'vars:' block
VARS_START=$(grep -n "^vars:" "$BLUEPRINT_FILE" | head -1 | cut -d: -f1)

if [ -z "$VARS_START" ]; then
	echo "ERROR: 'vars:' block not found in $BLUEPRINT_FILE" >&2
	exit 1
fi

# Find the next top-level key (line starting with non-whitespace, non-# character)
NEXT_SECTION=$(tail -n +$((VARS_START + 1)) "$BLUEPRINT_FILE" | grep -n "^[^ #]" | head -1 | cut -d: -f1)

if [ -z "$NEXT_SECTION" ]; then
	# No next section exists, search to the end of the file
	VARS_END=$(wc -l <"$BLUEPRINT_FILE")
else
	# Calculate the absolute line number for the end of the range (exclusive)
	VARS_END=$((VARS_START + NEXT_SECTION - 1))
fi

# 2. Check the state within the identified RANGE and apply changes
RANGE="${VARS_START},${VARS_END}"

# Check if 'labels:' exists specifically within the 'vars:' block
if ! sed -n "${RANGE}p" "$BLUEPRINT_FILE" | grep -q "^  labels:"; then
	# Case A: No labels in vars. Add it directly after the 'vars:' line.
	sed -i "${VARS_START}a\\  labels:\n    time-to-live: \"${TTL_LABEL}\"" "$BLUEPRINT_FILE"

elif ! sed -n "${RANGE}p" "$BLUEPRINT_FILE" | grep -q "time-to-live:"; then
	# Case B: labels: exists in vars, but time-to-live does not.
	# Find the line number of 'labels:' within vars
	LABELS_LINE=$(sed -n "${RANGE}p" "$BLUEPRINT_FILE" | grep -n "^  labels:" | cut -d: -f1)
	if [ -n "$LABELS_LINE" ]; then
		# Convert relative to absolute line number
		LABELS_ABS=$((VARS_START + LABELS_LINE - 1))
		sed -i "${LABELS_ABS}a\\    time-to-live: \"${TTL_LABEL}\"" "$BLUEPRINT_FILE"
	fi

else
	# Case C: time-to-live already exists in vars. Update its value.
	sed -i "${RANGE}s/time-to-live: \"[^\"]*\"/time-to-live: \"${TTL_LABEL}\"/" "$BLUEPRINT_FILE"
fi

echo "✓ TTL label added/updated in $BLUEPRINT_FILE"
