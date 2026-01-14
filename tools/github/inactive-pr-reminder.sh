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

set -euo pipefail

# This script finds pull requests that have been inactive for a certain
# period and posts reminders. It closes PRs that have been inactive for too long.

# --- Constants ---
REMINDER_MARKER="<!-- PR_INACTIVITY_REMINDER -->"
DAYS_INTERVAL=15
DAYS_TO_CLOSE=600 # Will be changed to 180 once the current backlog is cleared
TEAM_TO_TAG="@GoogleCloudPlatform/hpc-toolkit"

# --- Functions ---

# get_approval_status checks if a PR has a successful "multi-approvers" status check.
# Arguments:
#	$1: PR number.
#   $2: The JSON object for the statusCheckRollup of a PR.
# Outputs:
#   Writes descriptive messages to stderr.
#   Returns 0 if the PR is approved, 1 otherwise.
get_approval_status() {
	local pr_number=$1
	local status_checks_json=$2
	echo "Checking approval status for PR #${pr_number}..." >&2
	if echo "$status_checks_json" | jq -e 'any(.contexts[] | select(.context | contains("multi-approvers"))); .state == "SUCCESS")' >/dev/null; then
		echo "PR #${pr_number} has sufficient approvals." >&2
		return 0 # true
	fi
	echo "PR #${pr_number} does not have sufficient approvals." >&2
	return 1 # false
}

# calculate_inactivity returns the number of days a PR has been inactive.
# Arguments:
#   $1: PR number.
#   $2: The `updatedAt` timestamp string.
# Outputs:
#   Writes descriptive message to stderr.
#   Writes the number of inactive days to stdout.
calculate_inactivity() {
	local pr_number=$1
	local updated_at=$2
	local updated_at_seconds now_seconds inactive_seconds inactive_days
	updated_at_seconds=$(date -d "$updated_at" +%s)
	now_seconds=$(date +%s)
	inactive_seconds=$((now_seconds - updated_at_seconds))
	inactive_days=$((inactive_seconds / 86400))
	echo "PR #${pr_number} has been inactive for ${inactive_days} days." >&2
	echo "$inactive_days"
}

# close_pr_if_overdue closes a PR if it is past its closing threshold and unapproved.
# Arguments:
#   $1: PR number.
#   $2: PR author's username.
#   $3: Number of inactive days.
#   $4: Exit code (0 for approved, 1 for not approved) indicating approval status.
# Outputs:
#   Returns 0 if the PR was closed, 1 otherwise.
close_pr_if_overdue() {
	local pr_number=$1
	local pr_author=$2
	local inactive_days=$3
	local approval_status_code=$4

	if ((inactive_days > DAYS_TO_CLOSE)) && [[ "$approval_status_code" -eq 1 ]]; then
		echo "PR #${pr_number} is unapproved and has been inactive for more than ${DAYS_TO_CLOSE} days. Closing."
		gh pr comment "$pr_number" --body "@${pr_author} This PR was automatically closed after being inactive for more than ${DAYS_TO_CLOSE} days. ${REMINDER_MARKER}"
		gh pr close "$pr_number"
		return 0 # true
	fi
	return 1 # false
}

# send_reminder_if_needed sends a reminder comment if the PR is due for one.
# Arguments:
#   $1: PR number.
#   $2: PR author's username.
#   $3: Number of inactive days.
#   $4: JSON array of the PR's comments.
#   $5: Exit code (0 for approved, 1 for not approved) indicating approval status.
#   $6: String indicating if changes are requested.
# Outputs:
#   Writes log messages to stdout.
#   May call the GitHub API to comment on a PR.
send_reminder_if_needed() {
	local pr_number=$1
	local pr_author=$2
	local inactive_days=$3
	local comments_json=$4
	local approval_status_code=$5
	local changes_requested=$6

	local comments reminder_count expected_reminders
	comments=$(echo "$comments_json" | jq -r '.[].body')
	reminder_count=$(echo "$comments" | grep -c "$REMINDER_MARKER" || true)
	echo "Found ${reminder_count} reminder(s) for PR #${pr_number}."

	expected_reminders=$((inactive_days / DAYS_INTERVAL))

	if ((expected_reminders > reminder_count)); then
		echo "Expected ${expected_reminders} reminder(s), found ${reminder_count}. Sending a new reminder."
		if [[ "$approval_status_code" -eq 0 ]]; then
			gh pr comment "$pr_number" --body "This PR is approved and has been inactive for ${inactive_days} days. @${pr_author}, please merge it. ${REMINDER_MARKER}"
		else
			if [[ "$changes_requested" == "CHANGES_REQUESTED" ]]; then
				gh pr comment "$pr_number" --body "This PR has been inactive for ${inactive_days} days and has changes requested. @${pr_author}, please address the requested changes or close the PR if it's no longer needed. ${REMINDER_MARKER}"
			else
				gh pr comment "$pr_number" --body "This PR has been inactive for ${inactive_days} days and has no unresolved comments. ${TEAM_TO_TAG}, please review. ${REMINDER_MARKER}"
			fi
		fi
	else
		echo "No new reminder needed for PR #${pr_number}."
	fi
}

# process_pr is the main handler for a single pull request.
# Arguments:
#   $1: A JSON object representing a single pull request.
# Outputs:
#   Writes log messages to stdout.
process_pr() {
	local pr_json=$1

	# Efficiently extract all needed values from the JSON object at once.
	local pr_number is_draft pr_author updated_at review_decision comments_json status_checks_json
	# A tab character is used as the delimiter for `read`
	IFS=$'\t' read -r pr_number is_draft pr_author updated_at review_decision comments_json status_checks_json < <(
		echo "$pr_json" | jq -r '[.number, .isDraft, .author.login, .updatedAt, .reviewDecision, (.comments | tojson), (.statusCheckRollup | tojson)] | @tsv'
	)

	echo "---"
	echo "Checking PR #${pr_number}"

	if [[ "$is_draft" == "true" ]]; then
		echo "PR #${pr_number} is a draft, skipping."
		return
	fi

	local approval_status_code inactive_days
	get_approval_status "$pr_number" "$status_checks_json"
	approval_status_code=$?
	inactive_days=$(calculate_inactivity "$pr_number" "$updated_at")

	# return immediately if closing the PR, else check if a reminder is needed
	close_pr_if_overdue "$pr_number" "$pr_author" "$inactive_days" "$approval_status_code" && return
	send_reminder_if_needed "$pr_number" "$pr_author" "$inactive_days" "$comments_json" "$approval_status_code" "$review_decision"
}

# --- Main Logic ---
main() {
	echo "Fetching open pull requests..."
	# Fetch all necessary PR data in a single call, then pipe each PR as a
	# JSON object to the process_pr function.
	gh pr list --json number,updatedAt,isDraft,comments,author,reviewDecision,statusCheckRollup |
		jq -c '.[]' |
		while read -r pr_json; do
			process_pr "$pr_json"
		done
	echo "---"
	echo "All pull requests checked."
}

main
