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

tac .pre-commit-config.yaml | grep id: | cut -d ':' -f2 | sed -e 's/ //g' >hooks.txt
lines=$(wc -l <hooks.txt)
((sl = (lines + 3) / 3))
split -l "$sl" hooks.txt
echo go-unit-tests >>xaa && cat xaa xab | sort | uniq | xargs | sed -e 's/ /,/g' >hooks1.txt
echo go-unit-tests >>xab && cat xab xac | sort | uniq | xargs | sed -e 's/ /,/g' >hooks2.txt
echo go-unit-tests >>xac && cat xac xaa | sort | uniq | xargs | sed -e 's/ /,/g' >hooks3.txt
echo "created hooks1.txt hooks2.txt and hooks3.txt with three lists of hooks to skip"
