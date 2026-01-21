#!/usr/bin/env python3
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

# To run this script for all files in the repository, use:
# git ls-files | xargs python3 tools/update_license_year.py
# make sure to update "license string" in pkg/modulewriter/constants.go

import argparse
import re
import sys
from datetime import datetime

CURRENT_YEAR = datetime.now().year
COPYRIGHT_PATTERN = re.compile(
    r"(?i)(Copyright\s+)(\d{4})(\s+\"?Google LLC\"?)"
)

def update_license_year(file_path):
    """
    Updates the copyright year in a file's license header.

    Args:
        file_path (str): The path to the file.

    Returns:
        bool: True if the file was modified, False otherwise.
    """
    try:
        with open(file_path, "r", encoding="utf-8") as f:
            content = f.read()
    except UnicodeDecodeError:
        # Ignore binary files
        return False

    match = COPYRIGHT_PATTERN.search(content)
    if not match:
        # If no copyright notice, let addlicense handle it
        return False

    copyright_year = int(match.group(2))
    if copyright_year == CURRENT_YEAR:
        return False

    new_content = COPYRIGHT_PATTERN.sub(
        r"\g<1>{}\g<3>".format(CURRENT_YEAR), content, count=1
    )

    with open(file_path, "w", encoding="utf-8") as f:
        f.write(new_content)

    return True

def main():
    parser = argparse.ArgumentParser(
        description="Update copyright year in license headers."
    )
    parser.add_argument("files", nargs="*", help="Files to process.")
    args = parser.parse_args()

    for file_path in args.files:
        if update_license_year(file_path):
            print(f"Updated license year in {file_path}")

    return 0

if __name__ == "__main__":
    sys.exit(main())
