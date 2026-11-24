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

# Example usage:
#
# bash tools/hotfix-version-update.sh
# bash tools/hotfix-version-update.sh -b hotfix-v1.109.1
# bash tools/hotfix-version-update.sh -r GoogleCloudPlatform/cluster-toolkit
#

set -e -o pipefail

REPO="${HOTFIX_REPO:-GoogleCloudPlatform/cluster-toolkit}"
REMOTE_NAME="origin"

usage() {
	echo "Usage: $0 [-b <hotfix-vX.Y.Z>] [-r <owner/repo>]"
	echo "  -b: Optional hotfix branch name (must match hotfix-vX.Y.Z+1 for the next patch release)."
	echo "      If omitted, hotfix-vX.Y.Z+1 is created directly from main."
	echo "  -r: Optional GitHub repository slug (default: GoogleCloudPlatform/cluster-toolkit)."
	exit 1
}

require_cmd() {
	local cmd="$1"
	local msg="$2"
	if ! type -P "${cmd}" >/dev/null 2>&1; then
		echo "${msg}" >&2
		exit 1
	fi
}

require_cmd git "Must install git!"
require_cmd git-sed "Must install git-extras package for git sed functionality"
require_cmd gh "Must install GitHub CLI tool for command line API access"
require_cmd jq "Must install jq for JSON parsing"

if ! gh auth status >/dev/null 2>&1; then
	echo 'Must authenticate using "gh auth login"' >&2
	exit 1
fi

GITDIR=$(mktemp -d)
trap 'rm -rf "${GITDIR}"' EXIT

BRANCH_NAME=""

while getopts "b:r:h" opt; do
	case "${opt}" in
	b) BRANCH_NAME="${OPTARG}" ;;
	r) REPO="${OPTARG}" ;;
	h) usage ;;
	*) usage ;;
	esac
done

if [[ ! "${REPO}" =~ ^[^/]+/[^/]+$ ]]; then
	echo "ERROR: Repository '${REPO}' must be in 'owner/repo' format." >&2
	exit 1
fi

OLD_TAG=$(gh release view -R "${REPO}" --json tagName --jq '.tagName' 2>/dev/null || true)
if [ -z "${OLD_TAG}" ]; then
	OLD_TAG=$(gh release list -R "${REPO}" --exclude-drafts --exclude-pre-releases -L 1 --json tagName --jq '.[0].tagName // empty')
fi

tag_regex='^v([0-9]+)\.([0-9]+)\.([0-9]+)$'
if [[ ! "${OLD_TAG}" =~ $tag_regex ]]; then
	echo "ERROR: Latest published release tag '${OLD_TAG}' does not match vX.Y.Z." >&2
	exit 1
fi

OLD_MAJOR="${BASH_REMATCH[1]}"
OLD_MINOR="${BASH_REMATCH[2]}"
OLD_PATCH="${BASH_REMATCH[3]}"

NEW_MAJOR="${OLD_MAJOR}"
NEW_MINOR="${OLD_MINOR}"
NEW_PATCH=$((OLD_PATCH + 1))

NEW_VERSION="${NEW_MAJOR}.${NEW_MINOR}.${NEW_PATCH}"
NEW_TAG="v${NEW_VERSION}"
EXPECTED_BRANCH="hotfix-${NEW_TAG}"

if [ -z "${BRANCH_NAME}" ]; then
	BRANCH_NAME="${EXPECTED_BRANCH}"
else
	branch_regex='^hotfix-v[0-9]+\.[0-9]+\.[0-9]+$'
	if [[ ! "${BRANCH_NAME}" =~ $branch_regex ]]; then
		echo "ERROR: Hotfix branch '${BRANCH_NAME}' must match ^hotfix-v[0-9]+\\.[0-9]+\\.[0-9]+$ (expected '${EXPECTED_BRANCH}')." >&2
		echo "Release workflows derive the GitHub release tag directly from the branch name." >&2
		exit 1
	fi

	if [ "${BRANCH_NAME}" != "${EXPECTED_BRANCH}" ]; then
		echo "ERROR: Hotfix branch '${BRANCH_NAME}' does not match the next patch release '${EXPECTED_BRANCH}' (latest release is '${OLD_TAG}')." >&2
		exit 1
	fi
fi

# Warn if a regular release candidate PR is currently open so the hotfix is also merged into it.
OPEN_RC_PRS=$(gh pr list -R "${REPO}" --state open --json number,title,headRefName,baseRefName,url \
	--jq '[.[] | select(.headRefName | startswith("release-candidate-"))]')
OPEN_RC_COUNT=$(echo "${OPEN_RC_PRS}" | jq 'length')

if [ "${OPEN_RC_COUNT}" -gt 0 ]; then
	echo
	echo "WARNING: An in-flight release candidate PR is currently open:"
	echo "${OPEN_RC_PRS}" | jq -r '.[] | "  - #\(.number): \(.title) (\(.headRefName) -> \(.baseRefName)) \(.url)"'
	echo "Coordinate with the release owner and ensure '${NEW_TAG}' is also merged into the active release-candidate branch."
	echo
fi

V_BRANCH="version/${NEW_TAG}"

gh repo clone "${REPO}" "${GITDIR}" -- --filter=blob:none --no-checkout --origin "${REMOTE_NAME}"
cd "${GITDIR}"
git fetch "${REMOTE_NAME}" "refs/heads/main:refs/remotes/${REMOTE_NAME}/main"
# Fetch develop branch optionally, as it may not exist on forks or custom repositories.
git fetch "${REMOTE_NAME}" "refs/heads/develop:refs/remotes/${REMOTE_NAME}/develop" >/dev/null 2>&1 || true

if git ls-remote --exit-code --heads "${REMOTE_NAME}" "refs/heads/${BRANCH_NAME}" >/dev/null 2>&1; then
	git fetch "${REMOTE_NAME}" "refs/heads/${BRANCH_NAME}:refs/remotes/${REMOTE_NAME}/${BRANCH_NAME}"

	if ! git merge-base --is-ancestor "${REMOTE_NAME}/main" "${REMOTE_NAME}/${BRANCH_NAME}"; then
		echo "ERROR: Hotfix branch '${BRANCH_NAME}' is behind '${REMOTE_NAME}/main' or does not descend from 'main'." >&2
		exit 1
	fi

	if git rev-parse --verify --quiet "${REMOTE_NAME}/develop" >/dev/null 2>&1; then
		MERGE_BASE_DEVELOP=$(git merge-base "${REMOTE_NAME}/${BRANCH_NAME}" "${REMOTE_NAME}/develop")
		DEVELOP_ONLY_COMMITS=$(git rev-list --count "${REMOTE_NAME}/main..${MERGE_BASE_DEVELOP}")
		if [ "${DEVELOP_ONLY_COMMITS}" -gt 0 ]; then
			echo "ERROR: Hotfix branch '${BRANCH_NAME}' contains ${DEVELOP_ONLY_COMMITS} commit(s) from 'develop' that are not on 'main':" >&2
			git log --oneline -n 10 "${REMOTE_NAME}/main..${MERGE_BASE_DEVELOP}" >&2
			echo "Hotfix branches must be cut directly from 'main', never from 'develop'." >&2
			exit 1
		fi
	fi

	git switch -c "${BRANCH_NAME}" "${REMOTE_NAME}/${BRANCH_NAME}"
else
	echo "Creating hotfix branch '${BRANCH_NAME}' directly from 'main'..."
	git switch -c "${BRANCH_NAME}" "${REMOTE_NAME}/main"
	git push -u "${REMOTE_NAME}" "${BRANCH_NAME}"
fi

EXISTING_VERSION_PR=$(gh pr list -R "${REPO}" --state open --base "${BRANCH_NAME}" --head "${V_BRANCH}" --json url --jq '.[0].url // empty')

git switch -c "${V_BRANCH}" "${BRANCH_NAME}"
echo "Updating Toolkit version references from ${OLD_TAG} to ${NEW_TAG}..."
git ls-files -z '*.go' '*versions.tf' | xargs -0 git sed "v${OLD_MAJOR}\.${OLD_MINOR}\.${OLD_PATCH}" "${NEW_TAG}" --

VERSION_PR_URL=""
if git diff --quiet; then
	echo "Version references on '${BRANCH_NAME}' are already updated to ${NEW_TAG}; skipping version-bump PR."
elif [ -n "${EXISTING_VERSION_PR}" ]; then
	VERSION_PR_URL="${EXISTING_VERSION_PR}"
	echo "Open version-bump PR already exists: ${VERSION_PR_URL}"
else
	if git ls-remote --exit-code --heads "${REMOTE_NAME}" "refs/heads/${V_BRANCH}" >/dev/null 2>&1; then
		echo "ERROR: Remote branch '${V_BRANCH}' already exists without an open PR targeting '${BRANCH_NAME}'." >&2
		echo "Inspect or delete '${V_BRANCH}' on '${REMOTE_NAME}' before re-running." >&2
		exit 1
	fi
	git add -u
	echo "Creating branch '${V_BRANCH}' with version update to ${NEW_VERSION}..."
	git commit -m "Increase version to ${NEW_VERSION}"
	git push -u "${REMOTE_NAME}" "${V_BRANCH}"
	echo "Opening pull request to update ${BRANCH_NAME} to version ${NEW_VERSION}..."
	VERSION_PR_URL=$(gh pr create -R "${REPO}" --base "${BRANCH_NAME}" --head "${V_BRANCH}" \
		--label "release-chore" \
		--title "Update Toolkit release to ${NEW_TAG}" \
		--body "Update hotfix branch \`${BRANCH_NAME}\` from ${OLD_TAG} to ${NEW_TAG}.")
	echo "Version-bump PR created: ${VERSION_PR_URL}"
fi

EXISTING_MAIN_PR=$(gh pr list -R "${REPO}" --state open --base main --head "${BRANCH_NAME}" --json url --jq '.[0].url // empty')
AHEAD_OF_MAIN=$(git rev-list --count "${REMOTE_NAME}/main..${BRANCH_NAME}")

HOTFIX_MAIN_PR_URL=""
if [ -n "${EXISTING_MAIN_PR}" ]; then
	HOTFIX_MAIN_PR_URL="${EXISTING_MAIN_PR}"
	echo "Open hotfix PR to main already exists: ${HOTFIX_MAIN_PR_URL}"
elif [ "${AHEAD_OF_MAIN}" -gt 0 ]; then
	echo "Opening draft pull request from ${BRANCH_NAME} to main..."
	HOTFIX_MAIN_PR_URL=$(gh pr create -R "${REPO}" --draft --base main --head "${BRANCH_NAME}" \
		--label "release-bugfix" \
		--title "Hotfix release ${NEW_TAG}" \
		--body "Emergency hotfix release \`${NEW_TAG}\` cut from \`main\` (${OLD_TAG}).")
	echo "Draft hotfix PR to main created: ${HOTFIX_MAIN_PR_URL}"
else
	echo "NOTE: '${BRANCH_NAME}' currently has no commits ahead of 'main', so a draft PR to 'main' cannot be opened yet."
	echo "After merging '${V_BRANCH}' and your targeted fix into '${BRANCH_NAME}', open the draft PR to 'main':"
	echo "  gh pr create -R ${REPO} --draft --base main --head ${BRANCH_NAME} --label release-bugfix --title \"Hotfix release ${NEW_TAG}\" --body \"Emergency hotfix release ${NEW_TAG} cut from main (${OLD_TAG}).\""
fi

PR_NUM_HINT="<PR_NUM>"
if [ -n "${HOTFIX_MAIN_PR_URL}" ]; then
	PR_NUM_HINT="${HOTFIX_MAIN_PR_URL##*/}"
elif [ -n "${VERSION_PR_URL}" ]; then
	PR_NUM_HINT="${VERSION_PR_URL##*/}"
fi

echo
echo "=== Hotfix Lifecycle Next Steps (${NEW_TAG}) ==="
echo
echo "1. Run targeted hotfix tests (avoid --all on emergency patches):"
echo "   tools/cloud-build/babysit/run --pr ${PR_NUM_HINT} --names HOTFIX-test-<test_name> -c 1"
echo "   (or filter by module tag: tools/cloud-build/babysit/run --pr ${PR_NUM_HINT} --tags <module_tag> -c 1)"
echo
echo "2. Mark the ${BRANCH_NAME} -> main PR ready for review (triggers draft release notes) and merge it to main (publishes ${NEW_TAG})."
echo
echo "3. Run the OSS Exit Gate publish script after ${NEW_TAG} is tagged on main:"
echo "   git fetch origin --tags && bash tools/publish_release.sh \"${NEW_TAG}\" \"${NEW_TAG}\""
echo
echo "4. Immediately open the main -> develop back-merge PR:"
echo "   gh pr create -R ${REPO} --base develop --head main --label release-chore --title \"Back-merge hotfix ${NEW_TAG} from main into develop\" --body \"Back-merge emergency hotfix ${NEW_TAG} from main into develop.\""
if [ "${OPEN_RC_COUNT}" -gt 0 ]; then
	echo
	echo "5. Also merge/cherry-pick ${NEW_TAG} into the open release-candidate-* branch listed above."
fi
