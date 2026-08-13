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

# Trap function that runs the rescue playbook (terraform destroy)
# if the pod is terminated by Kueue, fails, or is cancelled.
# Expects variables: RUN_CLEANUP, DEPLOYMENT_NAME, ANSIBLE_PID
cleanup_pod() {
	local exit_code=$?
	trap - EXIT SIGTERM SIGINT ERR
	set +e

	if [ "${RUN_CLEANUP:-false}" = "false" ]; then
		exit $exit_code
	fi
	if [ $exit_code -eq 0 ]; then exit_code=1; fi

	echo ""
	echo "=========================================================================="
	echo "CAUGHT SIGTERM OR SCRIPT ERROR!"
	echo "Halting primary Ansible execution..."
	echo "=========================================================================="
	if [ -n "${ANSIBLE_PID:-}" ]; then
		kill -TERM "$ANSIBLE_PID" 2>/dev/null || true
		wait "$ANSIBLE_PID" 2>/dev/null || true
		echo "Waiting 15s for Terraform to release GCS backend state locks..."
		sleep 15
	fi

	echo ""
	echo "INITIATING RESCUE PLAYBOOK: Destroying leaked infrastructure for $DEPLOYMENT_NAME..."

	{
		echo "- hosts: localhost"
		echo "  tasks:"
		echo "  - ansible.builtin.include_tasks:"
		echo "      file: tools/cloud-build/daily-tests/ansible_playbooks/tasks/rescue_gcluster_failure.yml"
	} >/workspace/cleanup-playbook.yml

	ansible-playbook /workspace/cleanup-playbook.yml -e deployment_name="$DEPLOYMENT_NAME" -e workspace="/workspace" || true

	echo "Graceful cleanup finished."
	exit $exit_code
}
