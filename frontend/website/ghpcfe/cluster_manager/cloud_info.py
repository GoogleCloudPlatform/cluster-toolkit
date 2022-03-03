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


import json
from google.oauth2 import service_account
from google.cloud import storage as gcs
from google.cloud.billing_v1.services import cloud_catalog
import google.cloud.exceptions
import googleapiclient.discovery

import configparser
from collections import defaultdict
import archspec.cpu

import logging
logger = logging.getLogger(__name__)


gcp_machine_table = defaultdict(lambda: defaultdict(lambda: 'x86_64'), {
        # General Purpose
        'e2': defaultdict(lambda: "x86_64"),
        'n2': defaultdict(lambda: "cascadelake"),
        'n2d': defaultdict(lambda: "zen2"),
        'n1': defaultdict(lambda: "x86_64"),
        # Compute Optimized
        'c2': defaultdict(lambda: "cascadelake"),
        'c2d': defaultdict(lambda: "zen2"), # TODO: Should be zen3, but CentOS7 doesn't have a new enough kernel to recognize as such.
        't2d': defaultdict(lambda: "zen2"), # TODO: Should be zen3, but CentOS7 doesn't have a new enough kernel to recognize as such.
        # Memory Optimized
        'm2': defaultdict(lambda: "cascadelake"),
        'm1': defaultdict(lambda: "broadwell", {'megamem': 'skylake_avx512', 'ultramem': 'broadwell'}),
        # Accelerated
        'a2': defaultdict(lambda: "cascadelake"),
    })



def _get_arch_for_node_type_gcp(instance, default='x86_64'):
    (family, group, size) = instance.split('-')
    return gcp_machine_table[family][group]


def _get_gcp_client(credentials, service="compute", api_version="v1"):
    credInfo = json.loads(credentials)
    creds = service_account.Credentials.from_service_account_info(credInfo)
    return (
            credInfo["project_id"],
            googleapiclient.discovery.build(service, api_version, credentials=creds, cache_discovery=False)
            )

def _get_gcp_machine_types(credentials, region, zone):
    (project, client) = _get_gcp_client(credentials)

    req = client.machineTypes().list(project=project, zone=zone,
                                filter="isSharedCpu=False")
    
    resp = req.execute()
    if "items" not in resp:
        return []

    return {
        mt["name"]: {
            "name": mt["name"],
            "family": mt["name"].split('-')[0],
            "memory": mt["memoryMb"],
            "vCPU": mt["guestCpus"],
            "arch": _get_arch_for_node_type_gcp(mt["name"], default="x86_64")
        }
        for mt in resp["items"]
    }




def get_machine_types(cloudProvider, credentials, region, zone):
    if cloudProvider == "GCP":
        return _get_gcp_machine_types(credentials, region, zone)
    else:
        raise Exception("Unsupport Cloud Provider")


class MachineFamily():
    def __init__(self, name = None):
        self.name = name
        self.members = []
        self.arch_family = None

    @property
    def common_arch(self):
        return get_common_arch([m["arch"] for m in self.members])

    def addMember(self, machineInfo):
        memberArch = archspec.cpu.TARGETS[machineInfo["arch"]]
        if not self.name:
            self.name = machineInfo["family"]
        if not self.arch_family:
            self.arch_family = memberArch.family
        elif self.arch_family != memberArch.family:
            raise Exception(f"Mismatched architectures! {self.arch_family.name} != {memberArch.family.name}")
        self.members.append(machineInfo)


def get_machine_families(cloudProvider, credentials, region, zone):
    """ From list of machines, produce a family info array """
    machines = get_machine_types(cloudProvider, credentials, region, zone)
    families = defaultdict(MachineFamily)
    [families[m["family"]].addMember(m) for m in machines.values()]
    return families.values()


def _get_arch_ancestry(arch):
    ancestry = {arch.name}
    for p in arch.parents:
        ancestry.update(_get_arch_ancestry(p))
    return ancestry

def get_common_arch(archs):
    archs = [archspec.cpu.TARGETS[a] for a in archs]
    commonSet = set.intersection(*[_get_arch_ancestry(a) for a in archs])
    if not commonSet:
        return None
    return max([archspec.cpu.TARGETS[a] for a in commonSet]).name

def get_arch_ancestry(archName):
    arch = archspec.cpu.TARGETS[archName]
    res = [archName]
    if arch.family != arch:
        for x in arch.parents:
            res.extend(get_arch_ancestry(x.name))
    return res

def get_arch_family(arch):
    return archspec.cpu.TARGETS[arch].family.name

def sort_architectures(archNames):
    archs = [archspec.cpu.TARGETS[a] for a in archNames]
    return [x.name for x in sorted(archs)]



def _get_gcp_region_zone_info(credentials):
    (project, client) = _get_gcp_client(credentials)

    req = client.zones().list(project=project)
    results = defaultdict(list)
    while req is not None:
        resp = req.execute()
        for zone in resp['items']:
            region = '-'.join(zone["name"].split('-')[:-1])
            results[region].append(zone["name"])   
        req = client.zones().list_next(previous_request=req, previous_response=resp)
    return results

def get_region_zone_info(cloudProvider, credentials):
    if cloudProvider == "GCP":
        return _get_gcp_region_zone_info(credentials)
    else:
        raise Exception("Unsupport Cloud Provider")



def _get_gcp_subnets(credentials):
    (project, client) = _get_gcp_client(credentials)

    req = client.subnetworks().listUsable(project=project)
    results = req.execute()
    entries = results['items']
    subnets = []
    for entry in entries:
        # subnet in the form of https://www.googleapis.com/compute/v1/projects/<project>/regions/<region>/subnetworks/<name>
        tokens = entry['subnetwork'].split("/")
        region = tokens[8]
        subnet = tokens[10]
        # vpc in the form of https://www.googleapis.com/compute/v1/projects/<project>/global/networks/<name>
        tokens = entry['network'].split("/")
        vpc = tokens[9]
        # cidr in standard form xxx.xxx.xxx.xxx/yy
        cidr = entry['ipCidrRange']
        subnets.append([vpc, region, subnet, cidr])
    return subnets


def get_subnets(cloudProvider, credentials):
    if cloudProvider == "GCP":
        return _get_gcp_subnets(credentials)
    else:
        raise Exception("Unsupport Cloud Provider")


_gcp_services_list = None
_gcp_compute_sku_list = None

def _get_gcp_instance_pricing(credentials, region, zone, instance_type):
    global _gcp_services_list
    global _gcp_compute_sku_list

    creds = service_account.Credentials.from_service_account_info(json.loads(credentials))
    catalog = cloud_catalog.CloudCatalogClient(credentials=creds)
    # Step one:  Find the Compute Engine service
    if not _gcp_services_list:
        _gcp_services_list = [x for x in catalog.list_services() if "Compute Engine" == x.display_name]
    services = _gcp_services_list
    if len(services) != 1:
        raise Exception("Did not find Compute Engine Service")
    # Step two: Get all the SKUs associated with the Compute Engine service
    if not _gcp_compute_sku_list:
        _gcp_compute_sku_list = [x for x in catalog.list_skus(parent=services[0].name)]
    skus = [x for x in _gcp_compute_sku_list if region in x.service_regions]
    # To zero'th degree, pricing for an instance is made up of:
    #   # cores * Price/PerCore of instance semi-family
    #   # GB RAM * Price/GBhr of instance semi-family
    #   <OTHER THINGS - local SSD, GPUs, Tier 1 networking>  THESE ARE TODO
    #   # Disk Storage - We'll just assume a 20GB disk - that's what we're currently getting


    # Google's Billing API has SKUs, but the SKUs don't map to anything - you
    # can't get SKU info from the actual products. We have to look up sku's
    # with pricing info, and try to map the SKU's description to the actual
    # Compute infrastructure we're using.  We do have to look at the
    # "description" field, which feels hazardous and liable to change

    def get_disk_price(disk_size, skus):
        def disk_sku_filter(elem):
            if elem.category.resource_family != "Storage":
                return False
            if elem.category.resource_group != "PDStandard":
                return False
            if region not in elem.service_regions:
                return False
            if not elem.description.startswith("Storage PD Capacity"):
                # Filter out 'Regional Storage PD Capacity...'
                return False
            return True
        disk_sku = [x for x in skus if disk_sku_filter(x)]
        if len(disk_sku) != 1:
            raise Exception("Failed to find singular appropriate disk")
        disk_price_expression = disk_sku[0].pricing_info[0].pricing_expression
        unit_price = disk_price_expression.tiered_rates[0].unit_price.nanos * 1e-9
        disk_cost_per_month = disk_size * unit_price
        disk_cost_per_hr = disk_cost_per_month / (24*30)
        return disk_cost_per_hr

    def get_cpu_price(num_cores, instance_type, skus):
        instance_description_mapper = {
            'e2': 'E2 Instance Core',
            'n2d': 'N2D AMD Instance Core',
            'c2': 'Compute optimized Core',
            'c2d': 'C2D AMD Instance Core',
            't2d': 'T2D AMD Instance Core',
            'a2': 'A2 Instance Core',
            'm1': 'Memory-optimized Instance Core',  # ??
            'm2': 'Memory Optimized Upgrade Premium for Memory-optimized Instance Core',  # ??
            'n2': 'N2 Instance Core',
            'n1': 'Custom Instance Core', #??
        }
        instance_class = instance_type.split('-')[0]
        if instance_class not in instance_description_mapper.keys():
            raise NotImplementedError(f"Do not yet have a price mapping for instance type {instance_type}")

        def cpu_sku_filter(elem):
            if elem.category.resource_family != "Compute":
                return False
            if elem.category.resource_group != "CPU":
                return False
            if elem.category.usage_type != "OnDemand":
                return False
            if region not in elem.service_regions:
                return False
            if "Sole Tenancy" in elem.description:
                return False
            if not elem.description.startswith(instance_description_mapper[instance_class]):
                return False
            return True
        cpu_sku = [x for x in skus if cpu_sku_filter(x)]
        if len(cpu_sku) != 1:
            raise Exception("Failed to find singular appropriate cpu billing")
        cpu_price_expression = cpu_sku[0].pricing_info[0].pricing_expression
        unit_price = cpu_price_expression.tiered_rates[0].unit_price.nanos * 1e-9
        cpu_price_per_hr = num_cores * unit_price
        return cpu_price_per_hr
    
    def get_mem_price(num_gb, instance_type, skus):
        instance_description_mapper = {
            'e2': 'E2 Instance Ram',
            'n2d': 'N2D AMD Instance Ram',
            'c2': 'Compute optimized Ram',
            'c2d': 'C2D AMD Instance Ram',
            't2d': 'T2D AMD Instance Ram',
            'a2': 'A2 Instance Ram',
            'm1': 'Memory-optimized Instance Ram',  # ??
            'n2': 'N2 Instance Ram',
        }
        # TODO: Deal with 'Extended Instance Ram'
        instance_class = instance_type.split('-')[0]
        if instance_class not in instance_description_mapper.keys():
            raise NotImplementedError(f"Do not yet have a price mapping for instance type {instance_type}")

        def mem_sku_filter(elem):
            if elem.category.resource_family != "Compute":
                return False
            if elem.category.resource_group != "RAM":
                return False
            if elem.category.usage_type != "OnDemand":
                return False
            if region not in elem.service_regions:
                return False
            if "Sole Tenancy" in elem.description:
                return False
            if not elem.description.startswith(instance_description_mapper[instance_class]):
                return False
            return True
        mem_sku = [x for x in skus if mem_sku_filter(x)]
        if len(mem_sku) != 1:
            raise Exception("Failed to find singular appropriate RAM billing")
        mem_price_expression = mem_sku[0].pricing_info[0].pricing_expression
        unit_price = mem_price_expression.tiered_rates[0].unit_price.nanos * 1e-9
        ram_price_per_hr = num_gb * unit_price
        return ram_price_per_hr

    machine = _get_gcp_machine_types(credentials, region, zone)[instance_type]
    return get_cpu_price(machine["vCPU"], instance_type, skus) + get_mem_price(machine["memory"]/1024, instance_type, skus) + get_disk_price(20.0, skus)



def get_instance_pricing(cloudProvider, credentials, region, zone, instance_type):
    """ Return price per hour for an instance """
    if cloudProvider == "GCP":
        return _get_gcp_instance_pricing(credentials, region, zone, instance_type)
    else:
        raise Exception("Unsupport Cloud Provider")



def gcs_apply_bucket_acl(bucket, account, permission="roles/storage.objectViewer"):
    
    logger.info(f"Attempting to grant {permission} to gs://{bucket}/ for user {account}")
    client = gcs.Client()
    try:
        gcs_bucket = client.get_bucket(bucket)
        policy = gcs_bucket.get_iam_policy()
        for binding in policy.bindings:
            if binding['role'] == permission:
                binding['members'].add(account)
                break
        else:
            policy.bindings.append({'role': permission, 'members': set(account)})

        gcs_bucket.set_iam_policy(policy)

    except Exception as exc:
        logger.error("Failed to apply GCS Policy", exc_info=exc)       


def gcs_upload_file(bucket, path, contents, extra_acl=[]):
    logger.info(f"Attempting to upload to gs://{bucket}/{path if path else''}")
    client = gcs.Client() 
    gcs_bucket = client.bucket(bucket)
    blob = gcs_bucket.blob(path)
    blob.upload_from_string(contents)
    for acl in extra_acl:
        if acl.get('user') and acl.get('permission'):
            if permission in ['OWNER', 'READER', 'WRITER']:
                blob.acl.user(user).grant(permission)
        blob.acl.save()
    client.close()
    
def gcs_fetch_file(bucket, paths):
    client = gcs.Client() 
    gcs_bucket = client.bucket(bucket)
    results = {}
    for path in paths:
        try:
            logger.debug(f"Attempting to download from gs://{bucket}/{path if path else ''}")
            blob = gcs_bucket.blob(path)
            results[path] = blob.download_as_text()
        except google.cloud.exceptions.NotFound as nf:
            logger.info("Attempt failed (Not Found) to download {path}", exc_info=nf)
    client.close()
    return results

def gcs_get_blob(bucket, path):
    """Returns a blob object - it may or may not exist"""
    client = gcs.Client()
    gcs_bucket = client.bucket(bucket)
    return gcs_bucket.blob(path)


def get_gcp_workbench_region_zone_info(credentials, service="notebooks", api_version="v1"):

    (project, nb) = _get_gcp_client(credentials, service, api_version)
    request = nb.projects().locations().list(name=f'projects/{project}')
    result = request.execute()
    locations = [x['locationId'] for x in result['locations']]
    return locations

