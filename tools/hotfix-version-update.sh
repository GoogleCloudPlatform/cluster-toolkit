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
# bash hotfix-version-update.sh -b hotfix-branch-name
#

set -e -o pipefail

usage() {
	echo "Usage: $0 -b <hotfix-branch-name>"
	echo "  -b: The name of the hotfix branch (must start with 'hotfix-' and be based on main)"
	exit 1
}

if ! type -P git 1>/dev/null; then
	echo "Must install git!"
	exit 1
fi

if ! type -P git-sed 1>/dev/null; then
	echo "Must install git-extras package for git sed functionality"
	exit 1
fi

if ! type -P gh 1>/dev/null; then
	echo "Must install GitHub CLI tool for command line API access"
	exit 1
fi

if ! type -P jq 1>/dev/null; then
	echo "Must install jq for JSON parsing"
	exit 1
fi

if ! gh auth status; then
	echo 'Must authenticate using "gh auth login"'
	exit 1
fi

GITDIR=$(mktemp -d)
trap 'rm -rf ${GITDIR}' EXIT

BRANCH_NAME=""

while getopts "b:" opt; do
	case "${opt}" in
	b) BRANCH_NAME="${OPTARG}" ;;
	*) usage ;;
	esac
done

if [ -z "$BRANCH_NAME" ]; then
	usage
fi

if [[ ! "$BRANCH_NAME" =~ ^hotfix- ]]; then
	echo "Branch name for hotfix process should start with hotfix-"
	exit 1
fi

# Check if the branch exist and shares a history with main
# If the API returns a non-zero exit code, or the response is empty,
# it usually means the branch doesn't exist or is an orphan branch (no common history).
API_RESPONSE=$(gh api "repos/GoogleCloudPlatform/hpc-toolkit/compare/main...$BRANCH_NAME")
EXIT_CODE=$?

if [ $EXIT_CODE -ne 0 ] || [ -z "$API_RESPONSE" ]; then
	echo "hotfix branch $BRANCH_NAME should be branched out of main"
	exit 1
fi

BEHIND_BY=$(echo "$API_RESPONSE" | jq -r .behind_by)

# Check if the branch is behind main
# If 'behind_by' is greater than 0, main has moved forward since the branch was created.
if [ "$BEHIND_BY" -gt 0 ]; then
	echo "hotfix branch $BRANCH_NAME is behind main"
	exit 1
fi

OLD_TAG=$(gh release list -R GoogleCloudPlatform/hpc-toolkit -L 1 --json tagName --jq '.[] | .tagName')
OLD_MAJOR=$(echo "${OLD_TAG}" | cut -f1 -d. | sed 's,v,,')
OLD_MINOR=$(echo "${OLD_TAG}" | cut -f2 -d.)
OLD_PATCH=$(echo "${OLD_TAG}" | cut -f3 -d.)

NEW_MAJOR="${OLD_MAJOR}"
NEW_MINOR="${OLD_MINOR}"
NEW_PATCH=$((OLD_PATCH + 1))

NEW_VERSION="${NEW_MAJOR}.${NEW_MINOR}.${NEW_PATCH}"
NEW_TAG="v${NEW_VERSION}"

V_BRANCH="version/${NEW_TAG}"
REMOTE_NAME=origin

gh repo clone GoogleCloudPlatform/hpc-toolkit "${GITDIR}" -- --single-branch --branch "${BRANCH_NAME}" --depth 1 --origin "${REMOTE_NAME}"
cd "${GITDIR}"
git switch -c "${V_BRANCH}" "${BRANCH_NAME}"
echo "Creating new Toolkit version branch"
echo "converting old v${OLD_MAJOR}.${OLD_MINOR}.${OLD_PATCH} to new ${NEW_TAG}"
git sed "v${OLD_MAJOR}\.${OLD_MINOR}\.${OLD_PATCH}" "${NEW_TAG}" -- **/*.go **/versions.tf
git add -u
echo "Creating new branch with version update to ${NEW_VERSION}"
git commit -m "Increase version to ${NEW_VERSION}"
git push -u "${REMOTE_NAME}" "${V_BRANCH}"
echo "Opening pull request to update ${BRANCH_NAME} to version ${NEW_VERSION}"
gh pr create --base "${BRANCH_NAME}" --head "${V_BRANCH}" \
	--label "release-chore" \
	--title "Update Toolkit release to ${NEW_TAG}" \
	--body "Update hotfix ${BRANCH_NAME} to ${NEW_VERSION}"
echo
echo
echo
echo
echo
echo "Consider running the test babysitter using the pull request number from above:"
echo
echo "tools/cloud-build/babysit/run --pr <PR_NUM> --all -c 1"
