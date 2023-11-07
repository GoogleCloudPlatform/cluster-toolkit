#!/usr/bin/env python3
# Copyright 2023 Google LLC
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
import subprocess
from subprocess import CalledProcessError
from typing import List

DESCRIPTION = """
quota-check is a tool to verify that GCP project has enough quota across multiple regions and zones.
Usage:
tools/cloud-build/quota-check/check.py --project=<MY_PROJECT>
"""

LOCATIONS = {
    "us-central1": ["a", "c"],
    "us-west4": ["c"]
}


def _run_ghpc(args: List[str]) -> None:
    subprocess.run(["./ghpc " + " ".join(args)], shell=True, check=True, capture_output=True)

def _process_ghpc_output(serr: str) -> None:
    for l in serr.splitlines():
        if l.startswith("not enough quota"):
            print(l)

def _check_zone(project: str, region: str, zone: str) -> None:
    print(f"Checking {region=} {zone=}", end=" ")
    try:
        _run_ghpc([
            "expand", "tools/cloud-build/quota-check/bp.yaml", 
            "-l ERROR", # so validation will cause failure
            "--skip-validators='test_deployment_variable_not_used'", # this validator is false-positive and irrelevant
            f"--vars='project_id={project},{region=},{zone=}'",
        ])
    except CalledProcessError as e:
        print("FAIL")
        _process_ghpc_output(e.stderr.decode("utf-8"))
    else:
        print("OK")


def main() -> None:
    parser = argparse.ArgumentParser(description=DESCRIPTION)
    parser.add_argument("--project", help="The project ID.")

    args = parser.parse_args()
    assert args.project, DESCRIPTION

    
    try:
        _run_ghpc(["--version"]) # Smoke test
    except CalledProcessError as e:
        print(e.stderr.decode("utf-8"))
        exit(e.returncode)

    for region, suffixes in LOCATIONS.items():
        for suffix in suffixes:
            zone = f"{region}-{suffix}"
            _check_zone(args.project, region, zone)

if __name__ == '__main__':
    main()
