#!/usr/bin/env python3
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

import argparse
import sys
from pathlib import Path

import yaml

def validate_build_file(file_path: Path) -> bool:
    """Validates a daily test build file.

    Args:
        file_path: The path to the build file.

    Returns:
        True if the file is valid, False otherwise.
    """
    try:
        with open(file_path, "r") as f:
            data = yaml.safe_load(f)
    except yaml.YAMLError as e:
        print(f"Error parsing YAML file {file_path}: {e}", file=sys.stderr)
        return False

    if not isinstance(data, dict):
        print(f"Error: {file_path} is not a valid YAML dictionary.", file=sys.stderr)
        return False

    steps = data.get("steps")
    if not isinstance(steps, list):
        print(f"Error: 'steps' not found or not a list in {file_path}", file=sys.stderr)
        return False

    for step in steps:
        if isinstance(step, dict) and step.get("id") == "check_for_running_build":
            script = step.get("script")
            if not script:
                print(
                    f"Error: 'script' not found in 'check_for_running_build' step in {file_path}",
                    file=sys.stderr,
                )
                return False
            expected_script = f"tools/cloud-build/check_running_build.sh {file_path}"
            if script != expected_script:
                print(
                    f"Error: Invalid 'script' in 'check_for_running_build' step in {file_path}",
                    file=sys.stderr,
                )
                print(f"  Expected: {expected_script}", file=sys.stderr)
                print(f"  Got:      {script}", file=sys.stderr)
                return False
            return True

    print(
        f"Error: 'check_for_running_build' step not found in {file_path}",
        file=sys.stderr,
    )
    return False


def main():
    parser = argparse.ArgumentParser(
        description="Validates daily test build files."
    )
    parser.add_argument(
        "filenames",
        nargs="*",
        help="The files to validate.",
    )
    args = parser.parse_args()

    results = [validate_build_file(Path(filename)) for filename in args.filenames]
    if not all(results):
        sys.exit(1)


if __name__ == "__main__":
    main()
