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
"""
This tool automates some manual tasks for cleaning up failed builds.
When provided with the uri for a deployment folder this tool will:
- download the tar locally
- extract the tar into a deployment folder
- destroy the deployment
- remove the tar and deployment folder

Usage:
tools/cleanup-build.py gs://my-bucket/test-name/build.tgz
"""

import argparse
import os
import shutil
import tarfile
import shlex
import subprocess
import sys

# pip install google-cloud-storage
from google.cloud import storage

def cp_from_gcs(gcs_source_uri: str, local_destination_path: str, project_id: str) -> str:
    """Downloads a file from Google Cloud Storage to a local destination.
    Args:
        gcs_source_uri: The path to the file in Google Cloud Storage using the gs:// notation.
        local_destination_path: The local path to save the file to.
        project_id: The Google Cloud project ID.
    """

    storage_client = storage.Client(project=project_id)
    bucket = storage_client.bucket(gcs_source_uri.split("/")[2])
    path = "/".join(gcs_source_uri.split("/")[3:])
    filename = gcs_source_uri.split('/')[-1]
    blob = bucket.blob(path)
    destination = f"{local_destination_path}/{filename}"
    blob.download_to_filename(destination)
    return destination

def unpack_tgz(tar_file: str, destination_folder: str):
  with tarfile.open(tar_file, "r:gz") as tar:
    tar.extractall(destination_folder)

def gcluster_path() -> str:
    gcluster = "gcluster"
    if os.path.exists(gcluster):
        return f"./{gcluster}"
    if shutil.which(gcluster) is not None:
        return gcluster  # it's in PATH
    raise RuntimeError(f"Could not find {gcluster} in PATH or current directory")

def destroy(deployment_folder: str) -> bool:
    cmd = f"{gcluster_path()} destroy {deployment_folder} --auto-approve"
    process = subprocess.Popen(shlex.split(cmd), stdout=subprocess.PIPE)
    for line in iter(lambda: process.stdout.read(1), b""):
        sys.stdout.buffer.write(line)
    process.wait()

    if process.returncode:
        stdout, stderr = process.communicate()
        print(f'stdout: {stdout}')
        print(f'stderr: {stderr}\n\n')
        print(f"Deployment destroy failed. Command to manually destroy:\n{cmd}")
        return False

    print("Deployment destroyed")
    return True

def main(args: argparse.Namespace) -> None:
    print('Downloading tgz file')
    tgz_file = cp_from_gcs(args.gcs_tar_path, ".", args.project_id)

    print('Extracting tgz file')
    deployment_folder, _ = os.path.splitext(tgz_file)
    unpack_tgz(tgz_file, os.path.dirname(tgz_file))

    print('Destroying deployment')
    if destroy(deployment_folder):
        print('Cleaning up')
        os.remove(tgz_file)
        shutil.rmtree(deployment_folder)

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--project_id", type=str, default="hpc-toolkit-dev", help="Your Google Cloud project ID.")
    parser.add_argument("gcs_tar_path", help="The path to the GCS tar file.")

    main(parser.parse_args())
