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
DAYS_INTERVAL=7
DAYS_TO_CLOSE=21
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

# get_latest_activity_timestamp returns the timestamp of the latest human activity on a PR.
# Arguments:
#   $1: PR number.
#   $2: The JSON array of the PR's comments.
#   $3: The PR's `createdAt` timestamp string (initial creation time).
#   $4: The JSON array of the PR's latest reviews.
#   $5: The PR's `updatedAt` timestamp string.
# Outputs:
#   Writes the timestamp of the latest activity to stdout.
get_latest_activity_timestamp() {
	local pr_number=$1
	local comments_json=$2
	local pr_created_at=$3
	local latest_reviews_json=$4
	local updated_at=$5

	# Check if there are any bot reminders
	local latest_bot_reminder_timestamp
	latest_bot_reminder_timestamp=$(echo "$comments_json" | jq -r '
		map(select(.body | contains("<!-- PR_INACTIVITY_REMINDER -->"))) |
		map(select(.createdAt | type == "string")) |
		map(.createdAt) |
		max // null
	')

	local latest_human_activity_timestamp=""

	if [[ -n "$latest_bot_reminder_timestamp" && "$latest_bot_reminder_timestamp" != "null" ]]; then
		# If there is a bot reminder, check if PR updatedAt is significantly after the bot reminder.
		# We use 60 seconds as a grace period for API write latencies.
		local bot_reminder_seconds updated_at_seconds
		bot_reminder_seconds=$(date -d "$latest_bot_reminder_timestamp" +%s)
		updated_at_seconds=$(date -d "$updated_at" +%s)

		if ((updated_at_seconds > bot_reminder_seconds + 60)); then
			# There has been some activity since the bot's last reminder.
			# We can use updatedAt as the latest activity timestamp.
			latest_human_activity_timestamp="$updated_at"
		fi
	else
		# No bot reminders yet. The updatedAt is the true last activity timestamp.
		latest_human_activity_timestamp="$updated_at"
	fi

	# Fallback: calculate the latest explicit human activity before the bot reminder.
	if [[ -z "$latest_human_activity_timestamp" ]]; then
		latest_human_activity_timestamp="$pr_created_at" # Start with PR creation as baseline

		# Check latest non-bot comment activity
		local latest_comment_updated_at
		latest_comment_updated_at=$(echo "$comments_json" | jq -r '
			map(select(.body | contains("<!-- PR_INACTIVITY_REMINDER -->") | not)) |
			map(select(.createdAt | type == "string")) |
			map(.createdAt) |
			max // null
		')

		if [[ -n "$latest_comment_updated_at" && "$latest_comment_updated_at" != "null" ]]; then
			if [[ "$(date -d "$latest_human_activity_timestamp" +%s)" -lt "$(date -d "$latest_comment_updated_at" +%s)" ]]; then
				latest_human_activity_timestamp="$latest_comment_updated_at"
			fi
		fi

		# Check latest review activity
		local latest_review_updated_at
		latest_review_updated_at=$(echo "$latest_reviews_json" | jq -r '
			map(select(.createdAt | type == "string")) |
			map(.createdAt) |
			max // null
		')

		if [[ -n "$latest_review_updated_at" && "$latest_review_updated_at" != "null" ]]; then
			if [[ "$(date -d "$latest_human_activity_timestamp" +%s)" -lt "$(date -d "$latest_review_updated_at" +%s)" ]]; then
				latest_human_activity_timestamp="$latest_review_updated_at"
			fi
		fi

		# Check latest commit activity
		local latest_commit_updated_at
		if latest_commit_updated_at=$(gh pr view "$pr_number" --json commits --jq '.commits | map(select(.committedDate | type == "string")) | map(.committedDate) | max // null' 2>/dev/null); then
			if [[ -n "$latest_commit_updated_at" && "$latest_commit_updated_at" != "null" ]]; then
				if [[ "$(date -d "$latest_human_activity_timestamp" +%s)" -lt "$(date -d "$latest_commit_updated_at" +%s)" ]]; then
					latest_human_activity_timestamp="$latest_commit_updated_at"
				fi
			fi
		else
			echo "Warning: Failed to fetch commits for PR #${pr_number}." >&2
		fi
	fi

	echo "$latest_human_activity_timestamp"
}

# send_reminder sends an inactivity reminder comment.
# Arguments:
#   $1: PR number.
#   $2: PR author's username.
#   $3: Number of inactive days.
#   $4: Exit code (0 for approved, 1 for not approved) indicating approval status.
#   $5: String indicating if changes are requested.
#   $6: Reminder type ("first" or "second").
send_reminder() {
	local pr_number=$1
	local pr_author=$2
	local inactive_days=$3
	local approval_status_code=$4
	local changes_requested=$5
	local reminder_type=$6

	local prefix=""
	if [[ "$reminder_type" == "second" ]]; then
		prefix="This is a second reminder that "
	fi

	if [[ "$approval_status_code" -eq 0 ]]; then
		gh pr comment "$pr_number" --body "${prefix}this PR is approved and should be merged/closed. It has been inactive for ${inactive_days} days. @${pr_author}, please merge it. ${REMINDER_MARKER}"
	else
		if [[ "$changes_requested" == "CHANGES_REQUESTED" ]]; then
			gh pr comment "$pr_number" --body "${prefix}this PR has been inactive for ${inactive_days} days and has changes requested. @${pr_author}, please address the requested changes or close the PR if it's no longer needed. ${REMINDER_MARKER}"
		else
			gh pr comment "$pr_number" --body "${prefix}this PR has been inactive for ${inactive_days} days and has no unresolved comments. ${TEAM_TO_TAG}, please review. ${REMINDER_MARKER}"
		fi
	fi
}

# process_pr is the main handler for a single pull request.
# Arguments:
#   $1: A JSON object representing a single pull request.
# Outputs:
#   Writes log messages to stdout.
process_pr() {
	local pr_json=$1

	local pr_number pr_author review_decision created_at updated_at
	IFS=$'\t' read -r pr_number pr_author review_decision created_at updated_at < <(
		echo "$pr_json" | jq -r '[.number, .author.login, .reviewDecision, .createdAt, .updatedAt] | @tsv'
	)

	# complex JSON arrays use individual jq -c calls
	local comments_json
	comments_json=$(echo "$pr_json" | jq -c '.comments')
	local latest_reviews_json
	latest_reviews_json=$(echo "$pr_json" | jq -c '.latestReviews')

	echo "---"
	echo "Checking PR #${pr_number}"
	local approval_status_code
	approval_status_code=$(get_approval_status "$pr_number" "$pr_json")

	local latest_activity_timestamp
	latest_activity_timestamp=$(get_latest_activity_timestamp "$pr_number" "$comments_json" "$created_at" "$latest_reviews_json" "$updated_at")

	local updated_at_seconds now_seconds inactive_seconds inactive_days
	updated_at_seconds=$(date -d "$latest_activity_timestamp" +%s)
	now_seconds=$(date +%s)
	inactive_seconds=$((now_seconds - updated_at_seconds))
	inactive_days=$((inactive_seconds / 86400))
	echo "PR #${pr_number} has been inactive for ${inactive_days} days."

	local reminder_count
	reminder_count=$(echo "$comments_json" | jq -r --arg since "$latest_activity_timestamp" '
		map(select(.body | contains("<!-- PR_INACTIVITY_REMINDER -->"))) |
		map(select(.createdAt > $since)) |
		length
	')
	echo "Found ${reminder_count} reminder(s) since last human activity for PR #${pr_number}."

	# Logic:
	# Day 7 -> Reminder 1
	# Day 14 -> Reminder 2
	# Day 21 -> Close PR (only if unapproved)
	if ((inactive_days >= DAYS_TO_CLOSE)) && [[ "$reminder_count" -eq 2 ]]; then
		if [[ "$approval_status_code" -eq 1 ]]; then
			echo "PR #${pr_number} is unapproved and has been inactive for ${inactive_days} days. Closing."
			gh pr comment "$pr_number" --body "@${pr_author} This PR was automatically closed after being inactive for more than ${DAYS_TO_CLOSE} days. ${REMINDER_MARKER}"
			gh pr close "$pr_number"
		else
			echo "PR #${pr_number} is approved but has been inactive for ${inactive_days} days. It should be merged or closed."
		fi
	elif ((inactive_days >= 14)) && [[ "$reminder_count" -eq 1 ]]; then
		echo "Expected 2 reminders, found 1. Sending a second reminder."
		send_reminder "$pr_number" "$pr_author" "$inactive_days" "$approval_status_code" "$review_decision" "second"
	elif ((inactive_days >= DAYS_INTERVAL)) && [[ "$reminder_count" -eq 0 ]]; then
		echo "Expected 1 reminder, found 0. Sending a first reminder."
		send_reminder "$pr_number" "$pr_author" "$inactive_days" "$approval_status_code" "$review_decision" "first"
	else
		echo "No new action needed for PR #${pr_number}."
	fi
}

# --- Main Logic ---
main() {
	echo "Fetching all non-draft pull requests..."
	local attempt_num=1
	local -r MAX_ATTEMPTS=2
	local -r SLEEP_DELAY=5

	while [ "$attempt_num" -le "$MAX_ATTEMPTS" ]; do
		echo "Attempt $attempt_num of $MAX_ATTEMPTS to fetch PRs..."
		if pr_list_output=$(gh pr list --limit 100 --label "external" --draft=false --json number,createdAt,comments,author,reviewDecision,statusCheckRollup,latestReviews,updatedAt); then
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
