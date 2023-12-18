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

index=0
declare -a paths
for file in "$@"; do
	paths[index]=$(dirname "$file")
	((index += 1))
done

uniq_paths=$(echo "${paths[@]}" | tr ' ' '\n' | sort -u)

pre=$(git status --short --untracked-files=all | sort)

for path in $uniq_paths; do
	terraform-docs markdown --config .tfdocs-markdown.yaml "${path}" || {
		echo "Error generating docs for ${path}"
		exit 1
	}
done

post=$(git status --short --untracked-files=all | sort)

diff <(echo "${pre}") <(echo "${post}") || {
	echo "Error: git status has changed after running terraform-docs"
	exit 1
}
