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

function parse {
	echo "$1" | sed 's/[[:alpha:]]*//' | awk -F. '{ printf("%d%03d%03d%03d\n", $1,$2,$3,$4); }'
}

function check {
	if [ "$(parse "$1")" -ge "$(parse "$2")" ]; then
		echo "yes"
	else
		echo "no"
	fi
}

# NO-tests:
#check "1.2" "1.3" # no
#check "v1.2.9" "v1.3" # no
#check "go1.20.9" "go1.21"   # no
#check "go1.21" "1.22"  # no
#check "go1.21-20230317-RC01" "1.22" # no

# YES-tests:
#check "1.3" "1.2" # yes
#check "1.2.0" "1.2" # yes
#check "10.1" "9.9" # yes
#check "v1.3" "v1.2.9" # yes
#check "go1.21" "go1.20.9"  # yes
#check "go1.21" "1.20"  # yes
#check "go1.21-20230317-RC01" "1.3" # yes

if [[ -z $1 ]] || [[ -z $2 ]]; then
	echo "ERROR: invalid input, expected two arguments"
	exit 1
fi
check "$1" "$2"
