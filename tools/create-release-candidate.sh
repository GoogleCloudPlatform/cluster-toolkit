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

# Example usage (defaults to minor version release):
#
# bash create-release-candidate.sh
#
# Alternatively:
#
# bash create-release-candidate.sh -t patch
# bash create-release-candidate.sh -t minor
# bash create-release-candidate.sh -t major

set -e -o pipefail

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

if ! gh auth status; then
	echo 'Must authenticate using "gh auth login"'
	exit 1
fi

GITDIR=$(mktemp -d)
trap 'rm -rf ${GITDIR}' EXIT

while getopts "t:" opt; do
	case "${opt}" in
	t) ARG_TYPE="${OPTARG}" ;;
	*) ARG_TYPE="minor" ;;
	esac
done
TYPE="${ARG_TYPE:-minor}"

OLD_TAG=$(gh release list -R GoogleCloudPlatform/hpc-toolkit -L 1 --json tagName --jq '.[] | .tagName')
OLD_MAJOR=$(echo "${OLD_TAG}" | cut -f1 -d. | sed 's,v,,')
OLD_MINOR=$(echo "${OLD_TAG}" | cut -f2 -d.)
OLD_PATCH=$(echo "${OLD_TAG}" | cut -f3 -d.)

case "${TYPE}" in
major)
	NEW_MAJOR=$((OLD_MAJOR + 1))
	NEW_MINOR=0
	NEW_PATCH=0
	;;
minor)
	NEW_MAJOR="${OLD_MAJOR}"
	NEW_MINOR=$((OLD_MINOR + 1))
	NEW_PATCH=0
	;;
patch)
	NEW_MAJOR="${OLD_MAJOR}"
	NEW_MINOR="${OLD_MINOR}"
	NEW_PATCH=$((OLD_PATCH + 1))
	;;
*)
	echo 'The "-t" option must be set to "major", "minor", or "patch"'
	exit 1
	;;
esac

NEW_VERSION="${NEW_MAJOR}.${NEW_MINOR}.${NEW_PATCH}"
NEW_TAG="v${NEW_VERSION}"

RC_BRANCH=release-candidate
V_BRANCH="version/${NEW_TAG}"
REMOTE_NAME=origin

gh repo clone GoogleCloudPlatform/hpc-toolkit "${GITDIR}" -- --single-branch --branch develop --depth 1 --origin "${REMOTE_NAME}"
cd "${GITDIR}"
git switch -c "${RC_BRANCH}" develop
echo "Creating new Toolkit release-candidate branch"
git push -u "${REMOTE_NAME}" "${RC_BRANCH}"
git switch -c "${V_BRANCH}" "${RC_BRANCH}"
echo "converting old v${OLD_MAJOR}.${OLD_MINOR}.${OLD_PATCH} to new ${NEW_TAG}"
git sed "v${OLD_MAJOR}\.${OLD_MINOR}\.${OLD_PATCH}" "${NEW_TAG}" -- **/*.go **/versions.tf
git add -u
echo "Creating new branch with version update to ${NEW_VERSION}"
git commit -m "Increase version to ${NEW_VERSION}"
git push -u "${REMOTE_NAME}" "${V_BRANCH}"
echo "Opening pull request to update release-candidate to version ${NEW_VERSION}"
gh pr create --base "${RC_BRANCH}" --head "${V_BRANCH}" \
	--label "release-chore" \
	--title "Update Toolkit release to ${NEW_TAG}" \
	--body "Set release-candidate to version ${NEW_VERSION}"
echo
echo
echo
echo
echo
echo "Consider running the test babysitter using the pull request number from above:"
echo
echo "tools/cloud-build/babysit/run --pr <PR_NUM> --all -c 1"
