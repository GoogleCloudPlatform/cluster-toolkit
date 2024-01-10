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

"""Filesystem configuration and management"""

import json
import logging
import subprocess
from pathlib import Path

from ..models import GCPFilestoreFilesystem, Filesystem, FilesystemImpl

from . import utils

from website.settings import SITE_NAME

logger = logging.getLogger(__name__)


def write_filestore_yaml(fs: GCPFilestoreFilesystem, target_dir: Path) -> None:
    yaml_file = target_dir / "filesystem.yaml"
    project_id = json.loads(fs.cloud_credential.detail)["project_id"]
    # Get first (only) export
    export_name = fs.exports.first().export_name

    with yaml_file.open("w") as f:
        f.write(
            f"""
blueprint_name: {fs.name}

vars:
  project_id: {project_id}
  deployment_name: {fs.name}
  region: {fs.cloud_region}
  zone: {fs.cloud_zone}
  labels:
    created_by: {SITE_NAME}

deployment_groups:
- group: primary
  modules:
  - source: modules/file-system/filestore
    kind: terraform
    id: {fs.name}
    settings:
      filestore_share_name: {export_name[1:]}
      network_id: projects/{project_id}/global/networks/{fs.vpc.cloud_id}
      zone: {fs.cloud_zone}
      size_gb: {fs.capacity}
      filestore_tier: {fs.get_performance_tier_display()}
    outputs:
    - network_storage
"""
        )


def update_filesystem(fs: Filesystem) -> None:
    return create_filesystem(fs)


def create_filesystem(fs: Filesystem) -> None:
    target_dir = _base_dir_for_fs(fs)
    if not target_dir.is_dir():
        target_dir.mkdir(parents=True)

    # Create creds file
    with _get_credentials_file(fs).open("w") as fp:
        fp.write(fs.cloud_credential.detail)
        fp.write("\n")

    # Convert to our native type
    fs_impl = FilesystemImpl(fs.impl_type)
    if fs_impl == FilesystemImpl.GCPFILESTORE:
        fs = fs.gcpfilestorefilesystem
        write_filestore_yaml(fs, target_dir)
    else:
        raise NotImplementedError("No support yet for this filesystem")


def _run_ghpc(target_dir: Path) -> None:
    ghpc_path = "/opt/gcluster/hpc-toolkit/ghpc"

    try:
        logger.info("Invoking ghpc create")
        log_out_fn = target_dir / "ghpc_create_log.stdout"
        log_err_fn = target_dir / "ghpc_create_log.stderr"
        with log_out_fn.open("wb") as log_out:
            with log_err_fn.open("wb") as log_err:
                subprocess.run(
                    [ghpc_path, "create", "filesystem.yaml"],
                    cwd=target_dir,
                    stdout=log_out,
                    stderr=log_err,
                    check=True,
                )
    except subprocess.CalledProcessError as cpe:
        logger.error("ghpc exec failed", exc_info=cpe)
        # No logs from stdout/err - get dumped to files
        raise


def start_filesystem(fs: Filesystem) -> None:
    """Effectively, just 'terraform apply'"""
    fs.cloud_state = "cm"
    fs.save()
    try:
        _run_ghpc(_base_dir_for_fs(fs))
        extra_env = {
            "GOOGLE_APPLICATION_CREDENTIALS": _get_credentials_file(fs)
        }
        target_dir = _tf_dir_for_fs(fs)
        utils.run_terraform(target_dir, "init")
        utils.run_terraform(target_dir, "plan", extra_env=extra_env)

        logger.info("Invoking terraform apply for fs %s:%s", fs.id, fs.name)
        utils.run_terraform(target_dir, "apply", extra_env=extra_env)

        logger.info(
            "terraform apply complete, getting status for fs %s:%s",
            fs.id,
            fs.name,
        )
        (out_fn, _) = utils.run_terraform(
            target_dir, "output", arguments=["-json"]
        )
        with out_fn.open("r") as outputfp:
            results = json.load(outputfp)
            data = results[f"network_storage_{fs.name}"]["value"]
            fs.cloud_id = f"network_storage_{fs.name}"
            fs.hostname_or_ip = data["server_ip"]
            fs.cloud_state = "m"
            fs.save()
    except subprocess.CalledProcessError as cpe:
        fs.cloud_state = "nm"
        fs.save()
        logger.error("Terraform apply failed", exc_info=cpe)
        if cpe.stdout:
            logger.info("TF stdout:\n%s\n", cpe.stdout.decode("utf-8"))
        if cpe.stderr:
            logger.info("TF stderr:\n%s\n", cpe.stderr.decode("utf-8"))
        raise


def destroy_filesystem(fs: Filesystem) -> None:
    target_dir = _tf_dir_for_fs(fs)
    extra_env = {"GOOGLE_APPLICATION_CREDENTIALS": _get_credentials_file(fs)}
    utils.run_terraform(target_dir, "destroy", extra_env=extra_env)


def get_terraform_dir(fs: Filesystem) -> Path:
    # Just a wrapper to expose as "non-private"
    return _tf_dir_for_fs(fs)


def _base_dir_for_fs(fs: Filesystem) -> Path:
    config = utils.load_config()
    return config["baseDir"] / "fs" / f"fs_{fs.id}"


def _tf_dir_for_fs(fs: Filesystem) -> Path:
    return _base_dir_for_fs(fs) / f"{fs.name}" / "primary"


def _get_credentials_file(fs: Filesystem) -> Path:
    return _base_dir_for_fs(fs) / "cloud_credentials"
