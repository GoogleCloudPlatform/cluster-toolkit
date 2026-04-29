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

# This script caches metadata for all the github version releases. Used to backfill the data.
# Commands to run the script:
# chmod +x tools/cache_metadata/run_backfill.sh
# ./tools/cache_metadata/run_backfill.sh
# If hitting Github API rate limits, can create and use a Personal Authentication token by generating and exporting it.
# export GITHUB_TOKEN="ghp_YourGeneratedTokenHere..."

# To collect metadata for one specific version, run the following command:
# go run tools/cache_metadata/main.go -version <VERSION>

set -e

# Ensure we have the latest tags
git fetch --tags

# Build the tool once
go build -o backfill_tool tools/cache_metadata/main.go

# Loop through all tags matching the pattern "v*"
for version in $(git tag -l "v*" | sort -V); do
	echo "Running for $version..."
	./backfill_tool -version "$version"
done

# Clean up
rm backfill_tool

echo "Finished processing all tags."
