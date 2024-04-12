#!/usr/bin/env python3
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

import time
import subprocess
import os
import re

MAX_ACTIVE_BUILD_RETRIES = 3  # Adjust this as needed
RETRY_DELAY_SECONDS = 30

BUILD_ID = os.environ.get("BUILD_ID", "non-existent-build")

PROJECT_ID = os.environ.get("PROJECT_ID", None)
if not PROJECT_ID:
    result = subprocess.run(["gcloud", "config", "get-value", "project"], capture_output=True, text=True)
    if result.returncode == 0:
        PROJECT_ID = result.stdout
if not PROJECT_ID:
    print("PROJECT_ID must be defined")
    exit(1)

def check_active_builds():
    """Checks for active Cloud Builds, with retries"""
    for i in range(MAX_ACTIVE_BUILD_RETRIES):
        print("Checking for active builds...")
        result = subprocess.run([
            "gcloud", "builds", "list", "--project", PROJECT_ID,
            "--filter=id!=\"{}\"".format(BUILD_ID), "--ongoing"
        ], capture_output=True, text=True)

        if not result.stdout:
            return  # No active builds

        print(f"Active builds found, retrying after {RETRY_DELAY_SECONDS} seconds (attempt {i+1})...")
        time.sleep(RETRY_DELAY_SECONDS)

    print("There are active Cloud Build jobs. Skip clean up, may require re-run.")
    exit(1)


def delete_filestore_instances():
    """Deletes Filestore instances"""
    print("Getting list of filestore instances...")
    result = subprocess.run([
        "gcloud", "filestore", "instances", "list", "--project", PROJECT_ID
    ], capture_output=True, text=True)

    lines = result.stdout.splitlines()[2:]  # Skip header lines
    if not lines:
        print("No Filestore instances found")
        return

    print("Deleting Filestore instances...")
    for line in lines:
        name, location = line.split()[:2]
        print(f"Deleting {name} at {location}")
        subprocess.run([
            "gcloud", "--project", PROJECT_ID, "filestore", "instances", "delete",
            "--force", "--quiet", "--location", location, name
        ], check=True)


def disable_filestore_api():
    """Disables the Filestore API with error handling"""
    print("Disabling Filestore API...")
    subprocess.run(
            ["gcloud", "services", "disable", "file.googleapis.com", "--force", "--project", PROJECT_ID],
            check=True
    )


def enable_filestore_api():
    """Enables the Filestore API"""
    print("Re-enabling Filestore API...")
    subprocess.run(
        ["gcloud", "services", "enable", "file.googleapis.com", "--project", PROJECT_ID],
        check=True
    )


def delete_vpc_peerings():
    """Deletes Filestore VPC Peerings"""
    print("Getting list of Filestore VPC peerings...")
    result = subprocess.run(
        ["gcloud", "compute", "networks", "peerings", "list", "--project", PROJECT_ID,
         "--format=value(peerings.name,name)"],
        capture_output=True, text=True
    )

    lines = result.stdout.splitlines()
    if not lines:
        print("No Filestore VPC peerings found")
        return

    for line in lines:
        peerings, network = line.split()
        for peer in peerings.split(";"):
            if re.fullmatch("filestore-peer-[0-9]+", peer):
                print(f"Deleting peer '{peer}' from network '{network}'")
                subprocess.run(
                    ["gcloud", "--project", PROJECT_ID, "compute", "networks", "peerings", "delete",
                     "--network", network, peer],
                    check=True
                )


# Main Execution Logic
check_active_builds()
delete_filestore_instances()
disable_filestore_api()
try:
    delete_vpc_peerings()
finally:
    enable_filestore_api()
