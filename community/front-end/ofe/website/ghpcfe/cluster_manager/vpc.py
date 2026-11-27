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

"""VPC Configuration and provisioning"""

import contextlib
import fcntl
import logging
import os
import shutil
import subprocess
from pathlib import Path

from . import credentials
from . import utils
from ..models import VirtualNetwork, VirtualSubnet

logger = logging.getLogger(__name__)


def create_vpc(vpc: VirtualNetwork) -> None:
    """Set up the network's Terraform working directory and plan it.

    Safe to call while create_subnet sets up the same directory: whichever
    caller takes the directory lock first sets it up and runs init, validate
    and plan; a caller that then finds it set up does nothing more.
    """

    # Before anything is written: a network the credential cannot create
    # leaves no working directory behind
    cloud = _preflight(vpc, "create")
    with _vpc_dir_lock(vpc.id):
        _set_up_vpc_dir(vpc, cloud)


@contextlib.contextmanager
def _vpc_dir_lock(vpc_id: int):
    """Hold an exclusive lock on one network's working directory.

    The "Create VPC" background task and a subnet form saved straight after
    it both set up vpcs/vpc_<id> (create_vpc, create_subnet). The lock is a
    flock on vpcs/.vpc_<id>.lock, beside the directory it guards so that
    the directory's existence is never the lock. flock locks belong to the
    open file, so two threads of one server block each other as two server
    processes do, and the kernel releases the lock when its holder exits,
    so a crashed holder never blocks a later caller. The lock file is left
    in place: removing it could let two callers lock different files.
    """
    lock_dir = _tf_dir_for_vpc(vpc_id).parent
    lock_dir.mkdir(parents=True, exist_ok=True)
    with open(lock_dir / f".vpc_{vpc_id}.lock", "a", encoding="utf-8") as fp:
        fcntl.flock(fp, fcntl.LOCK_EX)
        try:
            yield
        finally:
            fcntl.flock(fp, fcntl.LOCK_UN)


# Written, renamed into place whole, only once init, validate and plan have
# succeeded in a network's working directory: its presence means set up
_VPC_DIR_READY = ".ofe-vpc-ready"
# Present from the start of a set-up until the ready marker is written, so a
# directory this code left part-way is never taken for one written by an
# earlier OFE version
_VPC_DIR_SETTING_UP = ".ofe-vpc-setting-up"


def _vpc_dir_is_set_up(target_dir: Path) -> bool:
    """Whether _set_up_vpc_dir finished in target_dir.

    A directory without the ready marker was left part-way, by a crash or a
    failed init, validate or plan, and is set up again.

    Directories written by earlier OFE versions have no markers. Those
    versions wrote terraform.tfvars and then ran init in the same call, and
    init creates .terraform, so a directory with both had init run in it;
    those versions never set a directory up a second time, so it is left
    alone as it was before the upgrade. One whose init or plan failed
    part-way is left alone too, as before, and its next Terraform command
    fails with Terraform's own error. One without .terraform never reached
    init and is set up again. This code writes the setting-up marker before
    anything else, so its own unfinished directories never pass this check.
    """
    if (target_dir / _VPC_DIR_READY).is_file():
        return True
    return (
        (target_dir / "terraform.tfvars").is_file()
        and (target_dir / ".terraform").is_dir()
        and not (target_dir / _VPC_DIR_SETTING_UP).exists()
    )


def _write_whole(path: Path, text: str) -> None:
    """Write text to path through a ".partial" file renamed into place, so
    path never exists part-written"""
    partial = path.with_name(path.name + ".partial")
    with open(partial, "w", encoding="utf-8") as fp:
        fp.write(text)
    os.replace(partial, path)


def _set_up_vpc_dir(vpc: VirtualNetwork, cloud: dict) -> None:
    """Write the network's working directory and run init, validate and
    plan in it, unless it is already set up. Call with _vpc_dir_lock held.
    """
    target_dir = _tf_dir_for_vpc(vpc.id)
    if _vpc_dir_is_set_up(target_dir):
        logger.info(
            "Working directory %s of network %s is already set up",
            target_dir,
            vpc.name,
        )
        return

    # Mark the set-up as unfinished before writing anything else; only the
    # ready marker, written at the end, clears it
    target_dir.mkdir(parents=True, exist_ok=True)
    (target_dir / _VPC_DIR_SETTING_UP).touch()

    # copy in TerraForm templates. A directory left part-way by a crash or
    # a failed Terraform command is completed in place: copying over it
    # rewrites the template files and keeps anything else.
    shutil.copytree(
        _tf_source_dir_for_vpc("GCP"), target_dir, dirs_exist_ok=True
    )

    # point Terraform at the shared state bucket before any init runs
    utils.write_backend_tf(
        target_dir, utils.terraform_state_prefix("vpc", vpc.id)
    )

    if not vpc.is_managed:
        # If VPC is not managed by us, don't create it, use a TF data provider
        tf_main = target_dir / "main.tf"
        tf_main.unlink()
        generate_vpc_tf_datablock(vpc, target_dir)

    project_id = vpc.cloud_credential.project_id

    # create Terraform variable definition file
    _write_whole(
        target_dir / "terraform.tfvars",
        f"""
region = "{vpc.cloud_region}"
zone = "{vpc.cloud_zone}"
project = "{project_id}"
""",
    )

    try:
        # init reads the state bucket, so it acts as the credential too
        utils.run_terraform(target_dir, "init", **cloud)
        utils.run_terraform(target_dir, "validate", **cloud)
        utils.run_terraform(target_dir, "plan", **cloud)

    except subprocess.CalledProcessError as err:
        # No ready marker: the next create_vpc or create_subnet sets the
        # directory up again
        logger.error("Terraform planning failed", exc_info=err)
        if err.stdout:
            logger.info("TF stdout:\n%s\n", err.stdout.decode("utf-8"))
        if err.stderr:
            logger.info("TF stderr:\n%s\n", err.stderr.decode("utf-8"))
        raise

    # Last: the directory is set up only once init, validate and plan have
    # succeeded
    _write_whole(
        target_dir / _VPC_DIR_READY, "init, validate and plan succeeded\n"
    )
    (target_dir / _VPC_DIR_SETTING_UP).unlink()


def start_vpc(vpc: VirtualNetwork) -> None:
    """Effectively, just 'terraform apply'.

    The apply and the state read-back run under the directory lock, so a
    subnet saved or deleted meanwhile (create_subnet, delete_subnet) waits
    for them instead of changing the files Terraform is reading. The
    preflight runs before the lock, as in create_vpc.
    """
    try:
        cloud = _preflight(vpc, "create")
    except credentials.CredentialError:
        # Nothing was created: back from "Creating" to "New"
        vpc.cloud_state = "nm"
        vpc.save()
        raise
    with _vpc_dir_lock(vpc.id):
        _apply_vpc(vpc, cloud)


def _apply_vpc(vpc: VirtualNetwork, cloud: dict) -> None:
    """Apply the network's working directory and read its state back. Call
    with _vpc_dir_lock held."""
    target_dir = _tf_dir_for_vpc(vpc.id)
    try:
        utils.run_terraform(target_dir, "apply", **cloud)
    except subprocess.CalledProcessError as err:
        logger.error("Terraform apply failed", exc_info=err)
        if err.stdout:
            logger.info("TF stdout:\n%s\n", err.stdout.decode("utf-8"))
        if err.stderr:
            logger.info("TF stderr:\n%s\n", err.stderr.decode("utf-8"))
        # A failed apply can leave some resources behind. "Unknown" keeps
        # Edit, Apply Cloud Changes and Destroy available instead of leaving
        # the network stuck in "Creating".
        vpc.cloud_state = "um"
        vpc.save()
        raise

    # The apply succeeded, so the network exists. Mark it managed before
    # reading the state back, so a failure there does not leave it stuck in
    # "Creating".
    vpc.cloud_state = "m"
    vpc.save()
    try:
        state = utils.get_terraform_state(target_dir, **cloud)
        vpc.cloud_id = state["outputs"]["vpc_id"]["value"]
        vpc.save()
        for subnet in vpc.subnets.all():
            if subnet.is_managed:
                subnet.cloud_id = state["outputs"][f"subnet-{subnet.id}"][
                    "value"
                ]
                subnet.cloud_state = "m"
                subnet.save()
    except (subprocess.CalledProcessError, RuntimeError, KeyError) as err:
        logger.error(
            "Terraform apply succeeded but reading its state failed",
            exc_info=err,
        )
        raise


def destroy_vpc(vpc: VirtualNetwork) -> None:
    """Effectively, just 'terraform destroy', under the directory lock as in
    start_vpc. The preflight runs before the lock."""
    target_dir = _tf_dir_for_vpc(vpc.id)
    try:
        cloud = _preflight(vpc, "destroy")
    except credentials.CredentialError:
        # Nothing was destroyed: back from "Destroying"
        vpc.cloud_state = "m" if _is_managed(vpc) else "i"
        vpc.save()
        raise
    with _vpc_dir_lock(vpc.id):
        utils.run_terraform(target_dir, "destroy", **cloud)
    vpc.cloud_state = "xm"
    vpc.save()


def create_subnet(subnet: VirtualSubnet) -> None:
    """Create a new Subnet in an existing VPC
    Uses adds a subnet file to the VPC TF directory.
    If no VPC TF directory exists, create one importing a
    data resource for the VPC itself.
    Under the same lock as create_vpc, so a subnet saved while the "Create
    VPC" task sets up the directory waits for it instead of setting it up a
    second time, and is not written into it during that task's plan.
    """
    vpc = subnet.vpc
    target_dir = _tf_dir_for_vpc(vpc.id)
    with _vpc_dir_lock(vpc.id):
        if not _vpc_dir_is_set_up(target_dir):
            # Before anything is written, as in create_vpc
            _set_up_vpc_dir(vpc, _preflight(vpc, "create"))
        template = target_dir / "subnet.tf.template"
        with open(template, "r", encoding="utf-8") as fp:
            subnet_template = fp.read()
        subnet_template = subnet_template.replace("{SUBNET_ID}", str(subnet.id))
        subnet_template = subnet_template.replace("{CIDR_TEXT}", str(subnet.cidr))
        subnet_template = subnet_template.replace("{PRIVATE_GOOGLE_ACCESS_ENABLED}", str(subnet.private_google_access_enabled).lower())
        fname = target_dir / f"subnet-{subnet.id}.tf"
        with open(fname, "w", encoding="utf-8") as fp:
            fp.write(subnet_template)


def delete_subnet(subnet: VirtualSubnet) -> None:
    """Removes the subnet from the VPC, under the directory lock, so it
    waits for an apply or destroy in progress (start_vpc, destroy_vpc)"""
    vpc_id = subnet.vpc.id
    target_dir = _tf_dir_for_vpc(vpc_id)
    with _vpc_dir_lock(vpc_id):
        fname = target_dir / f"subnet-{subnet.id}.tf"
        if fname.exists():
            fname.unlink()


def generate_vpc_tf_datablock(vpc: VirtualNetwork, target_dir: Path) -> Path:
    output_file = target_dir / "vpc.tf"
    if "GCP" in vpc.cloud_provider:
        dstype = "google_compute_network"
        key = "name"
    else:
        raise NotImplementedError(
            f"Cloud Provider {vpc.cloud_provider} not yet implemented"
        )
    with output_file.open("w") as fp:
        fp.write(
            f"""
data {dstype} "the_vpc" {{
    {key} = {vpc.cloud_id}
}}

"""
        )
    return output_file


def generate_subnet_tf_datablock(
    subnet: VirtualSubnet, target_dir: Path
) -> Path:
    output_file = target_dir / f"subnet-{subnet.id}.tf"
    if "GCP" in subnet.cloud_provider:
        dstype = "google_compute_subnetwork"
        key = "name"
    else:
        raise NotImplementedError(
            f"Cloud Provider {subnet.cloud_provider} not yet implemented"
        )
    with output_file.open("w") as fp:
        fp.write(
            f"""
data {dstype} "subnet_{subnet.id}" {{
    {key} = {subnet.cloud_id}
}}

"""
        )
    return output_file


def _is_managed(vpc: VirtualNetwork) -> bool:
    """Whether OFE created the network.

    While a network is being destroyed its cloud_state is "dm" whether OFE
    created it or imported it, so the working directory decides: create_vpc
    removes main.tf for an imported network.
    """
    target_dir = _tf_dir_for_vpc(vpc.id)
    if vpc.cloud_state == "dm" and target_dir.is_dir():
        return (target_dir / "main.tf").exists()
    return vpc.is_managed


def _preflight(vpc: VirtualNetwork, verb: str) -> dict:
    """Check the permissions `verb` (create or destroy) needs, and return the
    credential and operation for its Terraform runs"""
    if _is_managed(vpc):
        key = f"vpc_{verb}"
    else:
        key = "vpc_import" if verb == "create" else "vpc_import_destroy"
    operation = f"{verb} network {vpc.name}"
    credentials.preflight(
        vpc.cloud_credential, operation, credentials.permissions_for(key)
    )
    return {"credential": vpc.cloud_credential, "operation": operation}


def _tf_dir_for_vpc(vpc_id: int) -> Path:
    config = utils.load_config()
    return config["baseDir"] / "vpcs" / f"vpc_{vpc_id}"


def _tf_source_dir_for_vpc(cloud: str) -> Path:
    config = utils.load_config()
    return config["baseDir"] / "infrastructure_files" / "vpc_tf" / cloud
