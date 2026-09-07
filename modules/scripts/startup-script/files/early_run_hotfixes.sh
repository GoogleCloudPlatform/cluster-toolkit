#!/bin/bash
# Copyright 2026 Google LLC
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

# This script applies fixes to VMs that must occur early in boot. For example,
# when yum or apt repositories are misconfigured, preventing most package
# operations from completing successfully.

if [[ -f /etc/os-release ]]; then
	# shellcheck source=/dev/null
	source /etc/os-release
fi

if [[ "${PRETTY_NAME:-}" == "CentOS Linux 7 (Core)" ]]; then
	echo "Applying hotfixes for CentOS 7"
	if grep -q '^mirrorlist' /etc/yum.repos.d/CentOS-Base.repo 2>/dev/null; then
		echo "Removing mirrorlist from default CentOS 7 repositories"
		sed -i '/^mirrorlist/d' /etc/yum.repos.d/CentOS-Base.repo
	fi
	if grep -q '^#baseurl=http://mirror.centos.org' /etc/yum.repos.d/CentOS-Base.repo 2>/dev/null; then
		echo "Reconfiguring default CentOS 7 repositories to use CentOS Vault"
		sed -i 's,^#baseurl=http://mirror.centos.org/,baseurl=http://vault.centos.org/,' /etc/yum.repos.d/CentOS-Base.repo
	fi
fi

# Clean up decommissioned Parallelstore package repositories.
# Parallelstore package repositories on Artifact Registry (parallelstore-packages) were decommissioned
# and permanently deleted, returning HTTP 404 and breaking yum/dnf/apt package operations.
shopt -s nullglob

if [[ -d /etc/yum.repos.d ]]; then
	repos=(/etc/yum.repos.d/parallelstore*.repo)
	if ((${#repos[@]} > 0)); then
		echo "Removing decommissioned parallelstore yum repository configs: ${repos[*]}"
		rm -f "${repos[@]}"
	fi
	for repofile in /etc/yum.repos.d/*.repo; do
		if grep -q 'parallelstore-packages' "$repofile" 2>/dev/null; then
			echo "Removing repository with decommissioned parallelstore-packages: $repofile"
			rm -f "$repofile"
		fi
	done
fi

if [[ -d /etc/apt/sources.list.d ]]; then
	for listfile in /etc/apt/sources.list.d/*.list; do
		if grep -q 'parallelstore-packages' "$listfile" 2>/dev/null; then
			echo "Removing decommissioned parallelstore-packages from $listfile"
			sed -i '/parallelstore-packages/d' "$listfile"
			if [[ ! -s "$listfile" ]]; then
				rm -f "$listfile"
			fi
		fi
	done
fi

if [[ -f /etc/apt/sources.list ]] && grep -q 'parallelstore-packages' /etc/apt/sources.list 2>/dev/null; then
	echo "Removing decommissioned parallelstore-packages from /etc/apt/sources.list"
	sed -i '/parallelstore-packages/d' /etc/apt/sources.list
fi

cache_dirs=(/var/cache/dnf/parallelstore* /var/cache/yum/*/parallelstore*)
if ((${#cache_dirs[@]} > 0)); then
	rm -rf "${cache_dirs[@]}" 2>/dev/null || true
fi

shopt -u nullglob

exit 0
