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
import os
import secrets
import subprocess
import time
from pathlib import Path

from ..models import GCPFilestoreFilesystem, Filesystem, FilesystemImpl

from . import cmek
from . import utils

from website.settings import SITE_NAME

logger = logging.getLogger(__name__)


def write_filestore_yaml(fs: GCPFilestoreFilesystem, target_dir: Path) -> None:
    # A revoked key is final: the blueprint is not regenerated.
    cmek.assert_not_revoked(fs)
    yaml_file = target_dir / "filesystem.yaml"
    project_id = json.loads(fs.cloud_credential.detail)["project_id"]
    # Get first (only) export
    export_name = fs.exports.first().export_name

    tier = fs.get_performance_tier_display()

    # kms-key / kms-key-iam for this filestore, then `use: [cmek_key_iam]` on the
    # instance. The IAM module exports the key id as kms_key_name, which is the
    # filestore module's own input, so nothing is wired by hand here.
    settings = cmek.CmekSettings.from_config(utils.load_config())
    chosen = cmek.resource_settings(fs, settings)
    key_modules = ""
    key_use = ""
    # Store the effective choice, not the box as typed: enforcement may have
    # overridden it, and once deployed the stored value is what stands
    # (is_frozen), and what the keys page lists.
    if not chosen and fs.cmek_enabled:
        fs.cmek_enabled = False
        fs.save(update_fields=["cmek_enabled"])
    if chosen:
        if not chosen["key_name"]:
            # Cloud KMS never frees a key name, so a Filestore recreated under
            # the same name must not derive the same key name. The suffix
            # makes it unique; storing it below makes retries reuse it.
            chosen["key_name"] = cmek.derive_key_name(
                "filestore", f"{fs.name}-{secrets.token_hex(4)}")
        # Record what was deployed, so the detail page reports this key
        # rather than recomputing it from settings that may since have
        # changed. The derived name is not otherwise stored anywhere.
        deployed = cmek.resolve_created_key(chosen, settings, project_id)
        fs.cmek_enabled = True
        fs.cmek_key_name = deployed["key_name"]
        fs.cmek_key_ring_name = deployed["key_ring_name"]
        fs.cmek_key_project_id = deployed["key_project_id"]
        fs.save(update_fields=[
            "cmek_enabled", "cmek_key_name", "cmek_key_ring_name",
            "cmek_key_project_id"])
        ring_exists = cmek.ring_already_exists(
            fs.cloud_credential.detail, deployed, fs.cloud_zone)
        key_modules = cmek.key_modules_yaml(
            service="filestore",
            project_id=project_id,
            location=fs.cloud_zone,
            settings=settings,
            ring_exists=ring_exists,
            **chosen,
        ) + "\n"
        key_use = f"\n    use: [{cmek.KEY_IAM_MODULE_ID}]"

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
{key_modules}  - source: modules/file-system/filestore
    kind: terraform
    id: {fs.name}{key_use}
    settings:
      filestore_share_name: {export_name[1:]}
      network_id: projects/{project_id}/global/networks/{fs.vpc.cloud_id}
      zone: {fs.cloud_zone}
      size_gb: {fs.capacity}
      filestore_tier: {tier}
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


def _run_ghpc(target_dir: Path, cred_env: dict) -> None:
    ghpc_path = "/opt/gcluster/cluster-toolkit/ghpc"

    # env=cred_env alone replaces the ENTIRE subprocess
    # environment (not just adds to it), so ghpc previously ran with no
    # HOME/PATH/etc. and failed outright ("neither $XDG_CACHE_HOME nor
    # $HOME are defined"). Copy the real environment first, as
    # clusterinfo.py's _run_ghpc already does.
    env = os.environ.copy()
    env.update(cred_env)

    try:
        logger.info("Invoking ghpc create")
        log_out_fn = target_dir / "ghpc_create_log.stdout"
        log_err_fn = target_dir / "ghpc_create_log.stderr"
        with log_out_fn.open("wb") as log_out:
            with log_err_fn.open("wb") as log_err:
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
                    env=env
                )
    except subprocess.CalledProcessError as cpe:
        logger.error("ghpc exec failed", exc_info=cpe)
        # No logs from stdout/err - get dumped to files
        raise


def _filestore(fs: Filesystem):
    """The row that carries the encryption fields, for a Filestore.

    They live on the GCPFilestoreFilesystem child, not on Filesystem, and a
    save of the parent does not write them.
    """
    if isinstance(fs, GCPFilestoreFilesystem):
        return fs
    if FilesystemImpl(fs.impl_type) == FilesystemImpl.GCPFILESTORE:
        return fs.gcpfilestorefilesystem
    return None


def start_filesystem(fs: Filesystem) -> None:
    """Effectively, just 'terraform apply'"""
    filestore = _filestore(fs)
    # The record was loaded when the start was queued: a key revoked since
    # then is refused here, and not written back by the saves below.
    cmek.keep_flags_current(fs)
    try:
        cmek.assert_not_revoked(filestore)
    except cmek.CmekConfigError as err:
        # Before anything is reset, so the record keeps its cloud_state.
        fs.status_message = str(err)
        fs.save(update_fields=["status_message"])
        raise
    # Stored now, while the record still says deployed, so a Filestore
    # deployed before the flag existed stays frozen if this start fails.
    cmek.record_deployed(filestore)
    # the blueprint (filesystem.yaml) is produced by create_filesystem(). If
    # it is absent that step never completed, and running ghpc anyway produces
    # an opaque 'filesystem.yaml does not exist' that hides the real cause and
    # leaves the record looking un-started. Fail here instead, keeping the
    # record retryable.
    blueprint = _base_dir_for_fs(fs) / "filesystem.yaml"
    if not blueprint.is_file():
        msg = (
            f"Cannot start filesystem {fs.id} ({fs.name}): its blueprint has "
            f"not been generated ({blueprint} is missing), so the create step "
            "did not complete. Check the filesystem's log, then recreate it."
        )
        fs.cloud_state = "nm"
        fs.status_message = msg  # surface the reason on the detail page
        fs.save()
        raise RuntimeError(msg)
    fs.cloud_state = "cm"
    fs.status_message = ""  # clear any stale failure reason on a fresh start
    fs.save()
    try:
        extra_env = {
            "GOOGLE_APPLICATION_CREDENTIALS": _get_credentials_file(fs)
        }
        _run_ghpc(_base_dir_for_fs(fs), extra_env)

        target_dir = _tf_dir_for_fs(fs)
        utils.run_terraform(target_dir, "init")
        utils.run_terraform(target_dir, "plan", extra_env=extra_env)

        # No targeted key apply: consumers `use:` the kms-key-iam module, so
        # Terraform already creates the key and grants on it before anything
        # that encrypts with it.
        # Stored before the apply runs, not after it succeeds: a failed apply
        # can already have created the key or the share, so the choice stays
        # fixed whatever state a failed start or create leaves.
        cmek.record_apply_started(filestore)
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
            cmek.keep_flags_current(fs)
            fs.save()
    except subprocess.CalledProcessError as cpe:
        fs.cloud_state = "nm"
        # give the detail page a pointer to the real error.
        fs.status_message = (
            "The Terraform apply failed - see the Terraform log below for the "
            "provider error (for example an invalid capacity or quota issue)."
        )
        cmek.keep_flags_current(fs)
        fs.save()
        logger.error("Terraform apply failed", exc_info=cpe)
        if cpe.stdout:
            logger.info("TF stdout:\n%s\n", cpe.stdout.decode("utf-8"))
        if cpe.stderr:
            logger.info("TF stderr:\n%s\n", cpe.stderr.decode("utf-8"))
        raise
    except Exception as err:  # pylint: disable=broad-except
        # Anything that is not a Terraform failure: a generator defect, an API
        # error, a missing file. Without this the record keeps cloud_state "cm"
        # for good, so the page shows "being created" forever and the reason
        # only ever appears in django.log. Clusters already do this in
        # clusterinfo.start_cluster(); filesystems were the outlier.
        fs.cloud_state = "nm"
        fs.status_message = f"Deployment failed: {err}"
        cmek.keep_flags_current(fs)
        fs.save()
        logger.error("Filesystem %s:%s failed to start", fs.id, fs.name,
                     exc_info=err)
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
