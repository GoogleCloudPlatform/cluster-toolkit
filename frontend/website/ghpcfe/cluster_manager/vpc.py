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

from ..models import VirtualNetwork, VirtualSubnet

from . import utils

import logging
logger = logging.getLogger(__name__)

def create_vpc(vpc: VirtualNetwork) -> None:

    # create working directory and copy in TerraForm templates
    tgtDir = _tf_dir_for_vpc(vpc.id)
    if not tgtDir.parent.is_dir():
        tgtDir.parent.mkdir(parents=True)
    shutil.copytree(_tf_source_dir_for_vpc("GCP"), tgtDir)

    # create credential file
    with open(tgtDir / 'cloud_credentials', 'w') as fp:
        fp.write(vpc.cloud_credential.detail)
        fp.write("\n")

    project_id = json.loads(vpc.cloud_credential.detail)["project_id"]

    # create Terraform variable definition file
    with open(tgtDir / 'terraform.tfvars', 'w') as fp:
        fp.write(f"""
region = "{vpc.cloud_region}"
zone = "{vpc.cloud_zone}"
project = "{project_id}"
""")

    extraEnv = {'GOOGLE_APPLICATION_CREDENTIALS': Path(tgtDir / 'cloud_credentials').as_posix()}
    if not vpc.is_managed:
        # If VPC is not managed by us, don't create it, use a TF data provider
        mainTF = tgtDir / 'main.tf'
        mainTF.unlink()
        generate_vpc_tf_datablock(vpc, tgtDir)
    try:
        utils.run_terraform(tgtDir, "init")
        utils.run_terraform(tgtDir, "validate", extraEnv=extraEnv)
        utils.run_terraform(tgtDir, "plan", extraEnv=extraEnv)
    except subprocess.CalledProcessError as cpe:
        logger.error("Terraform exec failed", exc_info=cpe)
        if cpe.stdout:
            logger.info(f"  STDOUT:\n{cpe.stdout.decode('utf-8')}\n")
        if cpe.stderr:
            logger.info(f"  STDERR:\n{cpe.stderr.decode('utf-8')}\n")
        raise


def start_vpc(vpc: VirtualNetwork) -> None:
    """Effectively, just 'terraform apply'"""
    tgtDir = _tf_dir_for_vpc(vpc.id)
    try:
        extraEnv = {'GOOGLE_APPLICATION_CREDENTIALS': Path(tgtDir / 'cloud_credentials').as_posix()}
        utils.run_terraform(tgtDir, "apply", extraEnv=extraEnv)
        stateFile = tgtDir / 'terraform.tfstate'
        with stateFile.open('r') as statefp:
            state = json.load(statefp)
            if vpc.is_managed:
                vpc.cloud_id = state["outputs"]["vpc_id"]["value"]
                vpc.save()
            for subnet in vpc.subnets.all():
                if subnet.is_managed:
                    subnet.cloud_id = state["outputs"][f"subnet-{subnet.id}"]["value"]
                    subnet.cloud_state = 'm'
                    subnet.save()
    except subprocess.CalledProcessError as cpe:
        logger.error("Terraform apply failed", exc_info=cpe)
        if cpe.stdout:
            logger.info(f"  STDOUT:\n{cpe.stdout.decode('utf-8')}\n")
        if cpe.stderr:
            logger.info(f"  STDERR:\n{cpe.stderr.decode('utf-8')}\n")
        vpc.status = 'e'
        vpc.save()
        raise


def destroy_vpc(vpc: VirtualNetwork) -> None:
    tgtDir = _tf_dir_for_vpc(vpc.id)
    extraEnv = {'GOOGLE_APPLICATION_CREDENTIALS': Path(tgtDir / 'cloud_credentials').as_posix()}
    utils.run_terraform(tgtDir, "destroy", extraEnv=extraEnv)
    vpc.cloud_state = 'xm'
    vpc.save()


def create_subnet(subnet: VirtualSubnet) -> None:
    """Create a new Subnet in an existing VPC
    Uses adds a subnet file to the VPC TF directory.
    If no VPC TF directory exists, create one importing a
    data resource for the VPC itself.
    """
    tgtDir = _tf_dir_for_vpc(subnet.vpc.id)
    if not tgtDir.is_dir():
        create_vpc(subnet.vpc)
    template = tgtDir / 'subnet.tf.template'
    with open(template, 'r') as fp:
        templateStr = fp.read()
    templateStr = templateStr.replace("{SUBNET_ID}", str(subnet.id))
    templateStr = templateStr.replace("{CIDR_TEXT}", str(subnet.cidr))
    fname = tgtDir / f'subnet-{subnet.id}.tf'
    with open(fname, 'w') as fp:
        fp.write(templateStr)


def delete_subnet(subnet: VirtualSubnet) -> None:
    """Removes the subnet from the VPC"""
    tgtDir = _tf_dir_for_vpc(subnet.vpc.id)
    fname = tgtDir / f'subnet-{subnet.id}.tf'
    if fname.exists():
        fname.unlink()


def generate_vpc_tf_datablock(vpc: VirtualNetwork, tgtDir: Path) -> Path:
    outFile = tgtDir / 'vpc.tf'
    if "GCP" in vpc.cloud_provider:
        dstype = "google_compute_network"
        key = "name"
    else:
        raise NotImplementedError(f"Cloud Provider {vpc.cloud_provider} not yet implmeneted")
    with outFile.open('w') as fp:
        fp.write(f"""
data {dstype} "the_vpc" {{
    {key} = {vpc.cloud_id}
}}

""")
    return outFile


def generate_subnet_tf_datablock(subnet: VirtualSubnet, tgtDir: Path) -> Path:
    outFile = tgtDir / f"subnet-{subnet.id}.tf"
    if "GCP" in subnet.cloud_provider:
        dstype = "google_compute_subnetwork"
        key = "name"
    else:
        raise NotImplementedError(f"Cloud Provider {vpc.cloud_provider} not yet implmeneted")
    with outFile.open('w') as fp:
        fp.write(f"""
data {dstype} "subnet_{subnet.id}" {{
    {key} = {subnet.cloud_id}
}}

""")
    return outFile


def _tf_dir_for_vpc(vpc_id: int) -> Path:
    config = utils.load_config()
    return config["baseDir"] / 'vpcs' / f'vpc_{vpc_id}'


def _tf_source_dir_for_vpc(cloud: str) -> Path:
    config = utils.load_config()
    return config["baseDir"] / 'infrastructure_files' / 'vpc_tf' / cloud
