#!/bin/bash
# Copyright 2026 Google LLC
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

# Returns 0 if a retriable error (e.g. Terraform capacity error) is found in the log, 1 otherwise.
# This script is called by Kueue pipelines to determine if the job should be requeued.
LOG_FILE=$1
if [ -z "$LOG_FILE" ]; then
	echo "Usage: $0 <log_file>" >&2
	exit 2
fi

if [ ! -f "$LOG_FILE" ]; then
	echo "Error: Log file '$LOG_FILE' does not exist or is not a regular file." >&2
	exit 2
fi

# Define all retriable errors here.
# Note: "Couldn't find a zone to deploy" and "ERROR: ZONE not found" are not included here
# because find_available_zone.sh now internally loops and waits for zone capacity to maintain Kueue locks.
RETRIABLE_ERRORS="ZONE_RESOURCE_POOL_EXHAUSTED|does not have enough resources available|not enough resources available|stockout|os-login.*ssh-keys.*add|resourceInUseByAnotherResource|Error acquiring the state lock|412 Precondition Failed|conditionNotMet|RATE_LIMIT_EXCEEDED|Mutate requests per minute|Error 429|HTTP 429|429 Too Many Requests|Error 50[0-9]|Internal error|backendError|Service Unavailable|connection reset by peer|TLS handshake timeout|overlaps with the existing allocated IP range|Connection refused|Connection timed out|Failed to connect to the host via ssh"

if grep -q -i -E "$RETRIABLE_ERRORS" "$LOG_FILE"; then
	exit 0
else
	exit 1
fi
