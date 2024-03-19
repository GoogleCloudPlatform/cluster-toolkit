#!/bin/bash
# Copyright 2024 "Google LLC"
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

#
# Checks for serial output of a VM and saves it on a file until the VM is
# deleted.
#

# Check for parameters
if [ $# -lt 3 ]; then
	echo "Usage: $0 <VM_NAME> <PROJECT> <ZONE>"
	exit 1
fi

VM_NAME=$1
PROJECT=$2
ZONE=$3

OUTPUT_FILE="vm_serial_$(date +%Y%m%d_%H%M%S).log" # Dynamic filename with timestamp

# Main loop
while true; do
	output=$(gcloud compute instances get-serial-port-output "$VM_NAME" --zone "$ZONE" --project "$PROJECT" --port 1 2>/dev/null)

	# Check if output is not empty
	if [ -n "$output" ]; then
		echo "$output" >"$OUTPUT_FILE"
	else
		exit 0
	fi

	sleep 1 # Wait for 1 second
done
