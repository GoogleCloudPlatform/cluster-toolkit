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

A="modules/file-system/filestore/scripts/mount.sh"
B="modules/file-system/pre-existing-network-storage/scripts/mount.sh"
LIST1="${A} ${B}"

A="community/modules/file-system/nfs-server/scripts/install-nfs-client.sh"
B="modules/file-system/filestore/scripts/install-nfs-client.sh"
C="modules/file-system/pre-existing-network-storage/scripts/install-nfs-client.sh"
LIST2="${A} ${B} ${C}"

for i in 1 2; do
	[[ $i == 1 ]] && LIST="${LIST1}" || LIST="${LIST2}"
	for a in $LIST; do
		for b in $LIST; do
			if [[ -n $(git diff --no-index "$a" "$b") ]]; then
				echo "found diff between ${a} and ${b}"
				exit 1
			fi
		done
	done
done
