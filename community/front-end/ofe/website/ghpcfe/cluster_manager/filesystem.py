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
import subprocess
from pathlib import Path

from ..models import GCPFilestoreFilesystem, Filesystem, FilesystemImpl

from . import cmek
from . import utils

from website.settings import SITE_NAME

logger = logging.getLogger(__name__)


def _filestore_cmek_key(fs, tier):
    """the CMEK key for a Filestore instance, or None.

    Basic tiers cannot be CMEK-encrypted at all, so a key set on one is
    a mistake worth reporting rather than silently dropping. ENTERPRISE
    and REGIONAL instances are regional; other tiers are zonal, and the
    key's location has to match.
    """
    key = getattr(fs, "cmek_key", "") or None
    if not key:
        return None
    if tier.startswith("BASIC"):
        raise cmek.CmekConfigError(
            f"Filestore tier {tier} does not support CMEK",
            key_name=key,
            remediation=(
                "Use the ZONAL, REGIONAL or ENTERPRISE tier, or clear the "
                "CMEK key on this filesystem."
            ),
        )
    location = (fs.cloud_region if tier in ("ENTERPRISE", "REGIONAL")
                else fs.cloud_zone)
    return str(cmek.check_key_location(key, location))


def write_filestore_yaml(fs: GCPFilestoreFilesystem, target_dir: Path) -> None:
    yaml_file = target_dir / "filesystem.yaml"
    project_id = json.loads(fs.cloud_credential.detail)["project_id"]
    # Get first (only) export
    export_name = fs.exports.first().export_name

    tier = fs.get_performance_tier_display()
    cmek_key = _filestore_cmek_key(fs, tier)

    # Resolve the key and grant the Filestore service agent on it in the
    # blueprint, then refer to the IAM module rather than naming the key.
    # That is what orders the instance behind the grant; a literal key
    # name would encrypt it just the same and race with the grant.
    #
    # service-enablement covers a project where cloudkms is turned off
    # after this blueprint was generated. It does NOT remove the need for
    # the API to be on beforehand: _run_ghpc generates without
    # --validation-level WARNING, so test_apis_enabled is fatal and runs
    # before any module in the blueprint could enable anything. The admin
    # guide documents that prerequisite; loosening validation on this path
    # to paper over the ordering would disable every other check with it.
    cmek_modules = ""
    cmek_yaml = ""
    if cmek_key:
        parsed = cmek.parse_crypto_key_name(cmek_key)
        cmek_modules = f"""
  - id: services-api
    source: community/modules/project/service-enablement
    settings:
      gcp_service_list:
        - cloudkms.googleapis.com

  - id: {cmek.KEY_MODULE_ID}
    source: community/modules/security/pre-existing-kms-key
    settings:
      project_id: {parsed.project}
      location: {parsed.location}
      key_ring_name: {parsed.key_ring}
      key_name: {parsed.key}

  - id: {cmek.IAM_MODULE_ID}
    source: community/modules/security/kms-key-iam
    use: [{cmek.KEY_MODULE_ID}]
    settings:
      project_id: {project_id}
      service_agents: [filestore]
"""
        cmek_yaml = (
            f"\n      kms_key_name: "
            f"$({cmek.IAM_MODULE_ID}.kms_key_name)"
        )

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
  modules:{cmek_modules}
  - source: modules/file-system/filestore
    kind: terraform
    id: {fs.name}
    settings:
      filestore_share_name: {export_name[1:]}
      network_id: projects/{project_id}/global/networks/{fs.vpc.cloud_id}
      zone: {fs.cloud_zone}
      size_gb: {fs.capacity}
      filestore_tier: {tier}{cmek_yaml}
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


def start_filesystem(fs: Filesystem) -> None:
    """Effectively, just 'terraform apply'"""
    # the blueprint (filesystem.yaml) is produced by create_filesystem().
    # If it is absent, that step never completed. Running ghpc anyway
    # produces an opaque 'filesystem.yaml does not exist' that hides the
    # real cause and leaves the record looking un-started, so fail here
    # instead, keeping the record retryable.
    blueprint = _base_dir_for_fs(fs) / "filesystem.yaml"
    if not blueprint.is_file():
        msg = (
            f"Cannot start filesystem {fs.id} ({fs.name}): its blueprint has "
            f"not been generated ({blueprint} is missing), so the create step "
            "did not complete. Check the filesystem's status message, then "
            "recreate it."
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
        # give the detail page a pointer to the real error.
        fs.status_message = (
            "The Terraform apply failed - see the Terraform log below for the "
            "provider error (for example an invalid capacity or quota issue)."
        )
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
