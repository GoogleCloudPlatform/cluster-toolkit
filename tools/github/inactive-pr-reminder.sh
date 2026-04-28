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
#   $2: The full PR JSON object.
# Outputs:
#   Writes descriptive messages to stderr.
#   Returns 0 if the PR is approved, 1 otherwise.
get_approval_status() {
	local pr_number=$1
	local pr_json=$2
	# echo "Checking approval status for PR #${pr_number}..." >&2 # Keep this line for basic info
	local status_checks_json
	status_checks_json=$(echo "$pr_json" | jq -c '.statusCheckRollup')
	if echo "$status_checks_json" | jq -e '.[] | select(.name == "multi-approvers / multi-approvers" and .conclusion == "SUCCESS")' >/dev/null; then
		echo "PR #${pr_number} has sufficient approvals." >&2
		echo "0" # true
	else
		echo "PR #${pr_number} does not have sufficient approvals." >&2
		echo "1" # false
	fi
}

# calculate_inactivity returns the number of days a PR has been inactive based on human activity.
# Arguments:
#   $1: PR number.
#   $2: The JSON array of the PR's comments.
#   $3: The PR's `createdAt` timestamp string (initial creation time).
#   $4: The JSON array of the PR's latest reviews.
# Outputs:
#   Writes descriptive message to stderr.
#   Writes the number of inactive days to stdout.
calculate_inactivity() {
	local pr_number=$1
	local comments_json=$2
	local pr_created_at=$3
	local latest_reviews_json=$4

	local latest_human_activity_timestamp="$pr_created_at" # Start with PR creation as baseline

	# Check latest non-bot comment activity
	local latest_comment_updated_at
	latest_comment_updated_at=$(echo "$comments_json" | jq -r '
		map(select(.body | contains("<!-- PR_INACTIVITY_REMINDER -->") | not)) |
		map(select(.createdAt | type == "string")) | # Ensure createdAt is a string
		map(.createdAt) |
		max // null # Get the max, or null if the array is empty
	')

	if [[ -n "$latest_comment_updated_at" && "$latest_comment_updated_at" != "null" ]]; then
		if [[ "$(date -d "$latest_human_activity_timestamp" +%s)" -lt "$(date -d "$latest_comment_updated_at" +%s)" ]]; then
			latest_human_activity_timestamp="$latest_comment_updated_at"
		fi
	fi

	# Check latest review activity
	local latest_review_updated_at
	latest_review_updated_at=$(echo "$latest_reviews_json" | jq -r '
		map(select(.createdAt | type == "string")) | # Ensure createdAt is a string
		map(.createdAt) |
		max // null # Get the max, or null if the array is empty
	')

	if [[ -n "$latest_review_updated_at" && "$latest_review_updated_at" != "null" ]]; then
		if [[ "$(date -d "$latest_human_activity_timestamp" +%s)" -lt "$(date -d "$latest_review_updated_at" +%s)" ]]; then
			latest_human_activity_timestamp="$latest_review_updated_at"
		fi
	fi

	local updated_at_seconds now_seconds inactive_seconds inactive_days
	updated_at_seconds=$(date -d "$latest_human_activity_timestamp" +%s)
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

	local pr_number pr_author review_decision created_at
	IFS=$'\t' read -r pr_number pr_author review_decision created_at < <(
		echo "$pr_json" | jq -r '[.number, .author.login, .reviewDecision, .createdAt] | @tsv'
	)

	# complex JSON arrays use individual jq -c calls
	local comments_json
	comments_json=$(echo "$pr_json" | jq -c '.comments')
	local latest_reviews_json
	latest_reviews_json=$(echo "$pr_json" | jq -c '.latestReviews')

	echo "---"
	echo "Checking PR #${pr_number}"
	local approval_status_code inactive_days
	approval_status_code=$(get_approval_status "$pr_number" "$pr_json")
	inactive_days=$(calculate_inactivity "$pr_number" "$comments_json" "$created_at" "$latest_reviews_json")

	# return immediately if closing the PR, else check if a reminder is needed
	close_pr_if_overdue "$pr_number" "$pr_author" "$inactive_days" "$approval_status_code" && return
	send_reminder_if_needed "$pr_number" "$pr_author" "$inactive_days" "$comments_json" "$approval_status_code" "$review_decision"
}

# --- Main Logic ---
main() {
	echo "Fetching all non-draft pull requests..."
	local attempt_num=1
	local -r MAX_ATTEMPTS=2
	local -r SLEEP_DELAY=5

	while [ "$attempt_num" -le "$MAX_ATTEMPTS" ]; do
		echo "Attempt $attempt_num of $MAX_ATTEMPTS to fetch PRs..."
		if pr_list_output=$(gh pr list --limit 100 --label "external" --draft=false --json number,createdAt,comments,author,reviewDecision,statusCheckRollup,latestReviews); then
			break
		fi

		if [ "$attempt_num" -ge "$MAX_ATTEMPTS" ]; then
			echo "Failed to fetch PR numbers after $MAX_ATTEMPTS attempts. Exiting." >&2
			exit 1
		else
			echo "Failed to fetch PRs on attempt $attempt_num. Retrying in $SLEEP_DELAY seconds..." >&2
			sleep "$SLEEP_DELAY"
			attempt_num=$((attempt_num + 1))
		fi
	done

	echo "$pr_list_output" |
		jq -c '.[]' |
		while read -r pr_json; do
			process_pr "$pr_json"
		done

	echo "---"
	echo "All pull requests checked."
}

main
