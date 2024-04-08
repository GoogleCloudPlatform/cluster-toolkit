#!/bin/bash
# Copyright 2022 Google LLC
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

if [[ -z "${ZONE}" ]]; then
	echo "ZONE is unset"
	exit 1
fi
if [[ -z "${PROJECT_ID}" ]]; then
	echo "PROJECT_ID is unset"
	exit 1
fi
if [[ -z "${TIMEOUT}" ]]; then
	echo "TIMEOUT is unset"
	exit 1
fi
if [[ -z "${INSTANCE_NAMES}" ]]; then
	echo "INSTANCE_NAMES is unset"
	exit 1
fi

IFS=' ' read -r -a waiting_for_startup <<<"${INSTANCE_NAMES[@]}"
echo "Instance names:"
for instance_name in "${waiting_for_startup[@]}"; do
	echo "- $instance_name"
done

fetch_cmd() {
	instance_name=$1
	echo "gcloud compute instances get-serial-port-output ${instance_name} --port 1 --zone ${ZONE} --project ${PROJECT_ID}"
}

instance_status_cmd() {
	instance_name=$1
	echo "gcloud compute instances list --filter 'name=${instance_name}' --format 'value(status)' --project '${PROJECT_ID}' --zones '${ZONE}'"
}

failed=()
succeeded=()
remove_instance_names() {
	new_array=()
	for ((i = 0; i < "${#waiting_for_startup[@]}"; i++)); do
		current_name="${waiting_for_startup[$i]}"
		if ! [[ "${failed[*]}" =~ $current_name ]]; then
			new_array+=("$current_name")
		fi
		if ! [[ "${succeeded[*]}" =~ $current_name ]]; then
			new_array+=("$current_name")
		fi
	done
	waiting_for_startup=("${new_array[@]}")
}

now=$(date +%s)
deadline=$((now + TIMEOUT))
error_file=$(mktemp)
# Match string for all finish types of the old guest agent and successful
# finishes on the new guest agent
finish_line="startup-script exit status"
# Match string for failures on the new guest agent
finish_line_err="Script.*failed with error:"
# This specific text is monitored for in tests, do not change.
inspect_output_text="To inspect the startup script output, please run:"

while [[ $now -lt $deadline ]]; do

	for ((i = 0; i < "${#waiting_for_startup[@]}"; i++)); do
		instance_name="${waiting_for_startup[$i]}"

		# Get serial logs
		ser_log_cmd=$(fetch_cmd "$instance_name")

		# Check if we got serial logs successfully
		if ! ser_log=$(eval "$ser_log_cmd" 2>"${error_file}"); then
			# Failure - print out error and stop checking this instance
			cat "${error_file}"

			failed+=("$instance_name")
			unset "waiting_for_startup[$i]"

			continue
		fi

		# Success - try to find the line which signifies that the script completed
		final_line=$(echo "$ser_log" | grep "${finish_line}\|${finish_line_err}")
		ret_code=$?
		if [[ $ret_code -ne 0 ]]; then
			if [[ $ret_code -eq 1 ]]; then
				# Didn't find the final line
				echo "[$instance_name] Could not detect end of startup script. Sleeping."
			else
				# grep failed (we should never get here) - keep checking this instance
				echo "[$instance_name] The 'grep' command failed" 1>&2
			fi

			continue
		fi

		# Found the final line - get the status
		# This line checks for an exit code - the assumption is that there is a number
		# at the end of the line and it is an exit code
		status=$(sed -r 's/.*([0-9]+)\s*$/\1/' <<<"${final_line}" | uniq)
		if [[ "${status}" -eq 0 ]]; then
			echo "[$instance_name] startup-script finished successfully"
			# Stop checking this instance
			succeeded+=("$instance_name")
			unset "waiting_for_startup[$i]"
		else
			if [[ "${status}" -eq 1 ]]; then
				echo "[$instance_name] startup-script finished with errors, ${inspect_output_text}"
			else
				echo "[$instance_name] Invalid return status: '${status}'"
				echo "${inspect_output_text}"
				echo "$ser_log_cmd"
			fi
			# Stop checking this instance
			failed+=("$instance_name")
			unset "waiting_for_startup[$i]"
		fi
	done

	# Remove successes/failures from the array
	# remove_instance_names
	waiting_for_startup=("${waiting_for_startup[@]}")

	if ! [[ ${#waiting_for_startup[@]} -gt 0 ]]; then
		break
	fi

	# Sleep before checking all instances again
	sleep 5
done
echo

# Print status of instances
if [[ "${#succeeded[@]}" -gt 0 ]]; then
	echo "startup-script completed successfully on instances:"
	for instance_name in "${succeeded[@]}"; do
		echo "- $instance_name"
	done
	echo
fi

if [[ "${#failed[@]}" -gt 0 ]]; then
	echo "startup-script failed on instances:"
	for instance_name in "${failed[@]}"; do
		echo "- $instance_name"
	done
	echo
fi

if [[ "${#waiting_for_startup[@]}" -gt 0 ]]; then
	echo "startup-script timed out after ${TIMEOUT}s on instances:"
	for instance_name in "${waiting_for_startup[@]}"; do
		echo "- $instance_name"
	done
	echo
fi

echo "${inspect_output_text}"
fetch_cmd "<instance_name>"
