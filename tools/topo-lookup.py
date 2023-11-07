#!/usr/bin/env python3
# Copyright 2023 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
import argparse
from typing import Tuple, List

# Import the Compute Engine API
# pip install google-cloud-compute 
from google.cloud import compute_v1
from google.cloud.compute_v1 import Instance

DESCRIPTION = """
topo-lookup will lookup physical hosts and report their distribution.
Usage:
tools/topo-lookup.py --project_id=my-project --zone=us-central1-a --name_re=.*my-vm.*
"""

# Create a Compute Engine API client
client = compute_v1.InstancesClient()

def lookup_vm_matches(name_re: str, zone: str, project_id: str) -> List[Instance]:
    """Looks up all vm instances with a given name pattern."""
    request = compute_v1.ListInstancesRequest(
        project=project_id, zone=zone, max_results=10000, filter=f"name eq {name_re}"
    )
    return client.list(request)

def analyze(instances: List[Instance]) -> Tuple[int, int, int]:
    """
    Analyzes the distribution of physical hosts.
    Returns:
        A tuple of the number of clusters, racks, and hosts seen in the list of instances.
    """
    clusters, racks, hosts = set(), set(), set()
    for instance in instances:
        host = instance.resource_status.physical_host
        if not host:
            print(f"Warning: {instance.name} is not part of placement policy and has no physical host information.")
            continue
        split_host = host.split("/")
        clusters.add(split_host[1])
        racks.add(split_host[2])
        hosts.add(split_host[3])
    return len(clusters), len(racks), len(hosts)

def _dotify(current: str, previous: str) -> str:
    if current == previous:
        return "." * len(current)
    return current

def print_physical_hosts(instances: List[Instance]):
    """Prints the physical hosts of the instances."""
    physical_hosts = {instance.name: instance.resource_status.physical_host for instance in instances}
    print("Physical hosts: VM Name")
    prev_cluster, prev_rack, prev_host = None, None, None
    for vm_name, physical_host in sorted(physical_hosts.items(), key=lambda item: item[1]):
        if not physical_host: continue
        current_cluster, current_rack, current_host = physical_host.split("/")[1:]
        temp = current_cluster, current_rack, current_host
        current_cluster = _dotify(current_cluster, prev_cluster)
        current_rack = _dotify(current_rack, prev_rack)
        current_host = _dotify(current_host, prev_host)
        prev_cluster, prev_rack, prev_host = temp
        physical_host = f"/{current_cluster}/{current_rack}/{current_host}"
        print(f"{physical_host}: {vm_name}")
    print("")

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=DESCRIPTION)
    parser.add_argument("--name_re", help="The name pattern of the instances.")
    parser.add_argument("--project_id", help="The project ID.")
    parser.add_argument("--zone", help="The zone where the instances exist.")

    args = parser.parse_args()

    if args.name_re is None:
        args.name_re = input('Enter a name regex for instances: ')

    if args.project_id is None:
        args.project_id = input('Enter a project_id: ')

    if args.zone is None:
        args.zone = input('Enter a zone: ')

    matching_instances = lookup_vm_matches(args.name_re, args.zone, args.project_id)
    print_physical_hosts(matching_instances)
    
    # Note terminology matches that used in https://cloud.google.com/compute/docs/instances/use-compact-placement-policies#verify-vm-location
    # Internal terminology may vary
    clusters, racks, hosts = analyze(matching_instances)
    print("Summary: The VMs are spread across")
    print(f"Clusters: {clusters}, Racks: {racks}, Hosts: {hosts}")
