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

"""VPC configuration and provisioning.

A network OFE creates is a Cluster Toolkit blueprint of modules/network/vpc
(network, subnets, Cloud Router and NAT, firewall rules) and
modules/network/private-service-access, deployed with `gcluster deploy` and
removed with `gcluster destroy`.

Private service access belongs to the network, as in the earlier Terraform
template: a network has one Service Networking connection, so every Cloud
SQL instance on it shares the one this blueprint creates. Created by a
cluster blueprint instead, a second Cloud SQL cluster would collide with the
first's, and destroying a cluster would try to remove the connection while
Cloud SQL still held it.

Subnets are read from the database when the blueprint is written, so
editing subnets needs no cloud call until "Apply Cloud Changes".
"""

import json
import logging
import secrets
import subprocess
from pathlib import Path

import yaml

from . import utils
from ..models import VirtualNetwork, VirtualSubnet

from website.settings import SITE_NAME

logger = logging.getLogger(__name__)

_GROUP = "primary"
_BLUEPRINT = "vpc.yaml"
# Same rules as the earlier Terraform template: SSH from anywhere, and all
# traffic between private (RFC 1918) addresses, whatever the subnet ranges.
_INTERNAL_RANGES = ["10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"]
# GCP resource names are at most 63 characters, and the network's resources
# are named after it: the longest suffix is the vpc module's
# "-fw-allow-iap-ingress"/"-fw-allow-ssh-ingress" (21). So the network name
# (prefix + "-" + 6 hex) must stay within 63 - 21 = 42: a 35-character prefix.
_NAME_PREFIX_LEN = 35


def create_vpc(vpc: VirtualNetwork) -> None:
    """Prepare the network's folder and credential; nothing is created in
    the cloud until start_vpc."""
    base_dir = _base_dir(vpc)
    base_dir.mkdir(parents=True, exist_ok=True)
    with open(_credentials_file(vpc), "w", encoding="utf-8") as fp:
        fp.write(vpc.cloud_credential.detail)
        fp.write("\n")


def start_vpc(vpc: VirtualNetwork) -> None:
    """Write the blueprint, then `gcluster create` and `gcluster deploy`."""
    if not _credentials_file(vpc).exists():
        create_vpc(vpc)
    if not vpc.cloud_id:
        # Stable across re-applies; random so a network never collides with
        # one from another OFE in the same project.
        vpc.cloud_id = (
            f"{vpc.name[:_NAME_PREFIX_LEN]}-{secrets.token_hex(3)}"
        )
        vpc.save()
    subnets = [s for s in vpc.subnets.all() if s.is_managed]
    for subnet in subnets:
        subnet.cloud_id = _subnet_name(vpc, subnet)
        subnet.save()

    base_dir = _base_dir(vpc)
    (base_dir / _BLUEPRINT).write_text(
        yaml.safe_dump(blueprint(vpc, subnets), sort_keys=False),
        encoding="utf-8",
    )
    creds = _credentials_file(vpc)
    try:
        utils.run_gcluster(base_dir, "create", _BLUEPRINT, "-w",
                           credentials_file=creds)
        utils.run_gcluster(base_dir, "deploy", vpc.cloud_id, "--auto-approve",
                           credentials_file=creds)
    except subprocess.CalledProcessError as err:
        logger.error("gcluster deploy failed for VPC %s", vpc.id, exc_info=err)
        if err.stdout:
            logger.info("gcluster output:\n%s\n", err.stdout.decode("utf-8"))
        # A failed deploy can leave some resources behind. "Unknown" keeps
        # Edit, Apply Cloud Changes and Destroy available instead of leaving
        # the network stuck in "Creating".
        vpc.cloud_state = "um"
        vpc.save()
        raise

    vpc.cloud_state = "m"
    vpc.save()
    for subnet in subnets:
        subnet.cloud_state = "m"
        subnet.save()


def destroy_vpc(vpc: VirtualNetwork) -> None:
    # A network that was never applied has no deployment to destroy.
    if vpc.cloud_id and (_base_dir(vpc) / vpc.cloud_id).is_dir():
        try:
            utils.run_gcluster(_base_dir(vpc), "destroy", vpc.cloud_id,
                               "--auto-approve",
                               credentials_file=_credentials_file(vpc))
        except subprocess.CalledProcessError:
            # Some resources may remain. "Unknown" offers Destroy again
            # rather than leaving the network stuck in "Destroying".
            vpc.cloud_state = "um"
            vpc.save()
            raise
    else:
        logger.info("VPC %s was never deployed; nothing to destroy", vpc.id)
    vpc.cloud_state = "xm"
    vpc.save()


def blueprint(vpc: VirtualNetwork, subnets: list) -> dict:
    """The network's Cluster Toolkit blueprint."""
    project_id = json.loads(vpc.cloud_credential.detail)["project_id"]
    network = vpc.cloud_id
    return {
        "blueprint_name": f"vpc-{vpc.id}",
        "terraform_backend_defaults": {
            "type": "gcs",
            "configuration": {
                "bucket": utils.get_terraform_state_bucket(),
                "prefix": utils.terraform_state_prefix("vpc", vpc.id),
            },
        },
        "vars": {
            "project_id": project_id,
            "deployment_name": network,
            "region": vpc.cloud_region,
            "labels": {"created_by": SITE_NAME},
        },
        "deployment_groups": [{
            "group": _GROUP,
            "modules": [{
                "id": "network",
                "source": "modules/network/vpc",
                "settings": {
                    "network_name": network,
                    "subnetworks": [{
                        "subnet_name": s.cloud_id,
                        "subnet_region": s.cloud_region,
                        "subnet_ip": s.cidr,
                        "subnet_private_access":
                            bool(s.private_google_access_enabled),
                    } for s in subnets],
                    "allowed_ssh_ip_ranges": ["0.0.0.0/0"],
                    # Replaced by the wider rule below.
                    "enable_internal_traffic": False,
                },
            }, {
                # Named after the network (psconnect-<network>), which is
                # unique, by the module's default.
                "id": "private_service_access",
                "source": "modules/network/private-service-access",
                "use": ["network"],
            }, {
                # gcluster requires the vpc and private-service-access
                # modules to be used by another module of the blueprint;
                # this one holds OFE's internal traffic rule (network_name
                # comes from the vpc module, the PSA range from the other).
                "id": "internal_firewall",
                "source": "modules/network/firewall-rules",
                "use": ["network"],
                "settings": {
                    "use_subnetwork_data": False,
                    "ingress_rules": [_internal_firewall_rule(network)],
                },
            }],
        }],
    }


def _internal_firewall_rule(network: str) -> dict:
    return {
        "name": f"{network}-fw-allow-internal",
        "description": "allow traffic between private addresses and from "
                       "Google services on private service access",
        # The PSA range is usually within 10.0.0.0/8 already; listing it
        # keeps services such as Managed Lustre reachable if it is not.
        "source_ranges": _INTERNAL_RANGES
                         + ["$(private_service_access.cidr_range)"],
        "allow": [
            {"protocol": "tcp", "ports": ["0-65535"]},
            {"protocol": "udp", "ports": ["0-65535"]},
            {"protocol": "icmp"},
        ],
    }


def get_log_file(vpc: VirtualNetwork) -> Path:
    """The network's gcluster.log (every create, deploy and destroy)."""
    return _base_dir(vpc) / utils.GCLUSTER_LOG


# Service Networking's reason when a producer (Cloud SQL, for one) still
# holds the network's private service access connection.
_PSA_IN_USE = "FLOW_SN_DC_RESOURCE_PREVENTING_DELETE_CONNECTION"


def destroy_blocked_by_psa(vpc: VirtualNetwork) -> bool:
    """Whether the network's last gcluster command was a destroy that Service
    Networking refused because its private service access is still in use,
    typically by a Cloud SQL instance deleted only recently."""
    try:
        log = get_log_file(vpc).read_text(encoding="utf-8", errors="replace")
    except OSError:
        return False
    last = log.rpartition("\n$ gcluster ")[2]
    return last.startswith("destroy") and _PSA_IN_USE in last


def _subnet_name(vpc: VirtualNetwork, subnet: VirtualSubnet) -> str:
    return f"{vpc.cloud_id}-subnet-{subnet.id}"


def _base_dir(vpc: VirtualNetwork) -> Path:
    config = utils.load_config()
    return config["baseDir"] / "vpcs" / f"vpc_{vpc.id}"


def _credentials_file(vpc: VirtualNetwork) -> Path:
    return _base_dir(vpc) / "cloud_credentials"
