#!/usr/bin/env python3
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



import argparse
from pathlib import Path
import os, shutil, sys, subprocess
import json
import logging
logger = logging.getLogger(__name__)

from ..models import *

from . import utils


def write_filestore_yaml(fs: GCPFilestoreFilesystem, tgtDir: Path) -> None:
    ymlFile = tgtDir / 'filesystem.yaml'
    project_id = json.loads(fs.cloud_credential.detail)["project_id"]
    # Get first (only) export
    export_name = fs.exports.first().export_name

    with ymlFile.open('w') as f:
        f.write(f"""
blueprint_name: {fs.name}

vars:
  project_id: {project_id}
  deployment_name: {fs.name}
  region: {fs.cloud_region}
  zone: {fs.cloud_zone}
    
resource_groups:
- group: primary
  resources:
  - source: resources/file-system/filestore
    kind: terraform
    id: {fs.name}
    settings:
      filestore_share_name: {export_name[1:]}
      network_name: {fs.vpc.cloud_id}
      zone: {fs.cloud_zone}
      size_gb: {fs.capacity}
      filestore_tier: {fs.get_performance_tier_display()}
    outputs:
    - network_storage
""")



def update_filesystem(fs: Filesystem) -> None:
    return create_filesystem(fs)


def create_filesystem(fs: Filesystem) -> None:
    tgtDir = _base_dir_for_fs(fs)
    if not tgtDir.is_dir():
        tgtDir.mkdir(parents=True)

    # Create creds file
    with _get_credentials_file(fs).open('w') as fp:
        fp.write(fs.cloud_credential.detail)
        fp.write("\n")

    # Convert to our native type
    Impl = FilesystemImpl(fs.impl_type)
    if Impl == FilesystemImpl.GCPFILESTORE:
        fs = fs.gcpfilestorefilesystem
        write_filestore_yaml(fs, tgtDir)
    else:
        raise NotImplementedError("No support yet for this filesystem")


def _run_ghpc(tgtDir: Path) -> None:
    ghpc_path = utils.load_config()["baseDir"].parent.parent / 'ghpc'

    try:
        logger.info("Invoking ghpc create")
        log_out_fn = tgtDir / f"ghpc_create_log.stdout"
        log_err_fn = tgtDir / f"ghpc_create_log.stderr"
        with log_out_fn.open('wb') as log_out:
            with log_err_fn.open('wb') as log_err:
                subprocess.run([ghpc_path.as_posix(), 'create', 'filesystem.yaml'],
                    cwd=tgtDir,
                    stdout=log_out, stderr=log_err,
                    check=True)
    except subprocess.CalledProcessError as cpe:
        logger.error("ghpc exec failed", exc_info=cpe)
        # No logs from stdout/err - get dumped to files
        raise

def start_filesystem(fs: Filesystem) -> None:
    """Effectively, just 'terraform apply'"""
    fs.status = 'cm'
    fs.save()
    try:
        _run_ghpc(_base_dir_for_fs(fs))
        extraEnv = {'GOOGLE_APPLICATION_CREDENTIALS': _get_credentials_file(fs)}
        tgtDir = _tf_dir_for_fs(fs)
        utils.run_terraform(tgtDir, "init")
        utils.run_terraform(tgtDir, "plan", extraEnv=extraEnv)
        logger.info(f"Invoking terraform apply for fs {fs.id}")
        utils.run_terraform(tgtDir, "apply", extraEnv=extraEnv)
        logger.info(f"terraform apply complete, getting status for fs {fs.id}")
        (out_fn, err_fn) = utils.run_terraform(tgtDir, "output", arguments=["-json"])
        with out_fn.open('r') as outputfp:
            results = json.load(outputfp)
            data = results[f'network_storage_{fs.name}']['value']
            fs.cloud_id = f'network_storage_{fs.name}'
            fs.hostname_or_ip = data['server_ip']
            fs.cloud_state = 'm'
            fs.save()
    except subprocess.CalledProcessError as cpe:
        fs.cloud_state = 'nm'
        fs.save()
        logger.error("Terraform apply failed", exc_info=cpe)
        if cpe.stdout:
            logger.info(f"  STDOUT:\n{cpe.stdout.decode('utf-8')}\n")
        if cpe.stderr:
            logger.info(f"  STDERR:\n{cpe.stderr.decode('utf-8')}\n")
        raise


def destroy_filesystem(fs: Filesystem) -> None:
    tgtDir = _tf_dir_for_fs(fs)
    extraEnv = {'GOOGLE_APPLICATION_CREDENTIALS': _get_credentials_file(fs)}
    utils.run_terraform(tgtDir, "destroy", extraEnv=extraEnv)


def get_terraform_dir(fs: Filesystem) -> Path:
    # Just a wrapper to expose as "non-private"
    return _tf_dir_for_fs(fs)

def _base_dir_for_fs(fs: Filesystem) -> Path:
    config = utils.load_config()
    return config["baseDir"] / 'fs' / f'fs_{fs.id}'

def _tf_dir_for_fs(fs: Filesystem) -> Path:
    return _base_dir_for_fs(fs) / f'{fs.name}' / 'primary'

def _get_credentials_file(fs: Filesystem) -> Path:
    return _base_dir_for_fs(fs) / 'cloud_credentials'
