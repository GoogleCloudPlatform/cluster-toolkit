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

import json
import logging
import shutil
import subprocess
from pathlib import Path

from . import utils
from ..models import VirtualNetwork, VirtualSubnet

logger = logging.getLogger(__name__)


def create_vpc(vpc: VirtualNetwork) -> None:

    # create working directory and copy in TerraForm templates
    target_dir = _tf_dir_for_vpc(vpc.id)
    if not target_dir.parent.is_dir():
        target_dir.parent.mkdir(parents=True)
    shutil.copytree(_tf_source_dir_for_vpc("GCP"), target_dir)

    # create credential file
    with open(target_dir / "cloud_credentials", "w", encoding="utf-8") as fp:
        fp.write(vpc.cloud_credential.detail)
        fp.write("\n")

    project_id = json.loads(vpc.cloud_credential.detail)["project_id"]

    # create Terraform variable definition file
    with open(target_dir / "terraform.tfvars", "w", encoding="utf-8") as fp:
        fp.write(
            f"""
region = "{vpc.cloud_region}"
zone = "{vpc.cloud_zone}"
project = "{project_id}"
"""
        )

    extra_env = {
        "GOOGLE_APPLICATION_CREDENTIALS": Path(
            target_dir / "cloud_credentials"
        ).as_posix()
    }
    if not vpc.is_managed:
        # If VPC is not managed by us, don't create it, use a TF data provider
        tf_main = target_dir / "main.tf"
        tf_main.unlink()
        generate_vpc_tf_datablock(vpc, target_dir)

    try:
        utils.run_terraform(target_dir, "init")
        utils.run_terraform(target_dir, "validate", extra_env=extra_env)
        utils.run_terraform(target_dir, "plan", extra_env=extra_env)

    except subprocess.CalledProcessError as err:
        logger.error("Terraform planning failed", exc_info=err)
        if err.stdout:
            logger.info("TF stdout:\n%s\n", err.stdout.decode("utf-8"))
        if err.stderr:
            logger.info("TF stderr:\n%s\n", err.stderr.decode("utf-8"))
        raise


def start_vpc(vpc: VirtualNetwork) -> None:
    """Effectively, just 'terraform apply'"""
    target_dir = _tf_dir_for_vpc(vpc.id)
    try:
        extra_env = {
            "GOOGLE_APPLICATION_CREDENTIALS": Path(
                target_dir / "cloud_credentials"
            ).as_posix()
        }
        utils.run_terraform(target_dir, "apply", extra_env=extra_env)
        tf_state_file = target_dir / "terraform.tfstate"
        with tf_state_file.open("r") as statefp:
            state = json.load(statefp)
            if vpc.is_managed:
                vpc.cloud_id = state["outputs"]["vpc_id"]["value"]
                vpc.save()
            for subnet in vpc.subnets.all():
                if subnet.is_managed:
                    subnet.cloud_id = state["outputs"][f"subnet-{subnet.id}"][
                        "value"
                    ]
                    subnet.cloud_state = "m"
                    subnet.save()
    except subprocess.CalledProcessError as err:
        logger.error("Terraform apply failed", exc_info=err)
        if err.stdout:
            logger.info("TF stdout:\n%s\n", err.stdout.decode("utf-8"))
        if err.stderr:
            logger.info("TF stderr:\n%s\n", err.stderr.decode("utf-8"))
        vpc.status = "e"
        vpc.save()
        raise


def destroy_vpc(vpc: VirtualNetwork) -> None:
    target_dir = _tf_dir_for_vpc(vpc.id)
    extra_env = {
        "GOOGLE_APPLICATION_CREDENTIALS": Path(
            target_dir / "cloud_credentials"
        ).as_posix()
    }
    utils.run_terraform(target_dir, "destroy", extra_env=extra_env)
    vpc.cloud_state = "xm"
    vpc.save()


def create_subnet(subnet: VirtualSubnet) -> None:
    """Create a new Subnet in an existing VPC
    Uses adds a subnet file to the VPC TF directory.
    If no VPC TF directory exists, create one importing a
    data resource for the VPC itself.
    """
    target_dir = _tf_dir_for_vpc(subnet.vpc.id)
    if not target_dir.is_dir():
        create_vpc(subnet.vpc)
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
    """Removes the subnet from the VPC"""
    target_dir = _tf_dir_for_vpc(subnet.vpc.id)
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


def _tf_dir_for_vpc(vpc_id: int) -> Path:
    config = utils.load_config()
    return config["baseDir"] / "vpcs" / f"vpc_{vpc_id}"


def _tf_source_dir_for_vpc(cloud: str) -> Path:
    config = utils.load_config()
    return config["baseDir"] / "infrastructure_files" / "vpc_tf" / cloud
