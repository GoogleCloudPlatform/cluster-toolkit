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

"""Filesystem configuration and management"""

import json
import logging
import subprocess
from pathlib import Path

from ..models import GCPFilestoreFilesystem, Filesystem, FilesystemImpl

from . import credentials
from . import utils

from website.settings import SITE_NAME

logger = logging.getLogger(__name__)


def write_filestore_yaml(fs: GCPFilestoreFilesystem, target_dir: Path) -> None:
    yaml_file = target_dir / "filesystem.yaml"
    project_id = fs.cloud_credential.project_id
    # Get first (only) export
    export_name = fs.exports.first().export_name

    with yaml_file.open("w") as f:
        f.write(
            f"""
blueprint_name: {fs.name}

terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: {utils.get_terraform_state_bucket()}
    prefix: {utils.terraform_state_prefix("filesystem", fs.id)}

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

    # Convert to our native type
    fs_impl = FilesystemImpl(fs.impl_type)
    if fs_impl == FilesystemImpl.GCPFILESTORE:
        fs = fs.gcpfilestorefilesystem
        write_filestore_yaml(fs, target_dir)
    else:
        raise NotImplementedError("No support yet for this filesystem")


def _run_ghpc(target_dir: Path, credential) -> None:
    ghpc_path = "/opt/gcluster/cluster-toolkit/ghpc"

    try:
        logger.info("Invoking ghpc create")
        log_out_fn = target_dir / "ghpc_create_log.stdout"
        log_err_fn = target_dir / "ghpc_create_log.stderr"
        # The gcluster validators read only a key file. tool_environment
        # starts from the server's environment, so ghpc keeps HOME.
        with credentials.tool_environment(
            credential, key_file=True
        ) as cred_env, log_out_fn.open("wb") as log_out, log_err_fn.open(
            "wb"
        ) as log_err:
            subprocess.run(
                # -w: overwrite an existing deployment folder from a
                # prior attempt (matches clusterinfo.py's ghpc create
                # invocation) — without it, any retry after a failed
                # create_filesystem()/start_filesystem() permanently
                # fails with "deployment folder already exists" until
                # an admin manually deletes it.
                [ghpc_path, "create", "filesystem.yaml", "-w"],
                cwd=target_dir,
                stdout=log_out,
                stderr=log_err,
                check=True,
                env=cred_env
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
        credential = fs.cloud_credential
        operation = f"create filesystem {fs.name}"
        cloud = {"credential": credential, "operation": operation}
        try:
            credentials.preflight(
                credential,
                operation,
                credentials.permissions_for("filesystem_create"),
            )
        except credentials.CredentialError as err:
            fs.cloud_state = "nm"
            fs.save()
            # Also on the filesystem's page, in its Terraform log
            credentials.record_refusal(_tf_dir_for_fs(fs), err, "apply")
            raise
        _run_ghpc(_base_dir_for_fs(fs), credential)

        target_dir = _tf_dir_for_fs(fs)
        # init and output read the state bucket, so they act as the
        # credential too
        utils.run_terraform(target_dir, "init", **cloud)
        utils.run_terraform(target_dir, "plan", **cloud)

        logger.info("Invoking terraform apply for fs %s:%s", fs.id, fs.name)
        utils.run_terraform(target_dir, "apply", **cloud)

        logger.info(
            "terraform apply complete, getting status for fs %s:%s",
            fs.id,
            fs.name,
        )
        (out_fn, _) = utils.run_terraform(
            target_dir, "output", arguments=["-json"], **cloud
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
    operation = f"destroy filesystem {fs.name}"
    try:
        credentials.preflight(
            fs.cloud_credential,
            operation,
            credentials.permissions_for("filesystem_destroy"),
        )
    except credentials.CredentialError as err:
        # Nothing was destroyed: back from "Destroying" to managed, or to
        # imported for a filesystem OFE did not create (no Terraform dir)
        managed = target_dir.is_dir()
        fs.cloud_state = "m" if managed else "i"
        fs.save()
        if managed:
            # Also on the filesystem's page, in its Terraform log. An
            # imported filesystem's page shows no log.
            credentials.record_refusal(target_dir, err, "destroy")
        raise
    utils.run_terraform(
        target_dir,
        "destroy",
        credential=fs.cloud_credential,
        operation=operation,
    )


def get_terraform_dir(fs: Filesystem) -> Path:
    # Just a wrapper to expose as "non-private"
    return _tf_dir_for_fs(fs)


def _base_dir_for_fs(fs: Filesystem) -> Path:
    config = utils.load_config()
    return config["baseDir"] / "fs" / f"fs_{fs.id}"


def _tf_dir_for_fs(fs: Filesystem) -> Path:
    return _base_dir_for_fs(fs) / f"{fs.name}" / "primary"
