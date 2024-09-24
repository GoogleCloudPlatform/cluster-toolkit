#!/usr/bin/env python3

# Copyright 2024 Google LLC
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
"""
This script sorts nodes based on their `physicalHost`.

See https://cloud.google.com/compute/docs/instances/use-compact-placement-policies

You can reduce latency in tightly coupled HPC workloads (including distributed ML training) 
by deploying them to machines that are located close together. 
For example, if you deploy your workload on a single physical rack, you can expect lower latency 
than if your workload is spread across multiple racks. 
Sending data across multiple rack requires sending data through additional network switches. 

Example usage:
``` my_sbatch.sh
#SBATCH --ntasks-per-node=8
#SBATCH --nodes=64

export SLURM_HOSTFILE=$(sort_nodes.py)

srun -l hostname | sort
```
"""
import os
import subprocess
import uuid
from typing import List, Optional, Dict
from collections import OrderedDict

def order(paths: List[List[str]]) -> List[str]:
    """
    Orders the leaves of the tree in a way that minimizes the sum of distance in between 
    each pair of neighboring nodes in the resulting order.
    The resulting order will always start from the first node in the input list.
    The ordering is "stable" with respect to the input order of the leaves i.e.
    given a choice between two nodes (identical in other ways) it will select "nodelist-smallest" one.

    Returns a list of nodenames, ordered as described above.  
    """
    if not paths: return []
    class Vert:
        "Represents a vertex in a *network* tree."
        def __init__(self, name: str, parent: "Vert"):
            self.name = name
            self.parent = parent
            # Use `OrderedDict` to preserve insertion order
            # TODO: once we move to Python 3.7+ use regular `dict` since it has the same guarantee
            self.children = OrderedDict()

    # build a tree, children are ordered by insertion order
    root = Vert("", None)
    for path in paths:
        n = root
        for v in path:
            if v not in n.children:
                n.children[v] = Vert(v, n)
            n = n.children[v]

    # walk the tree in insertion order, gather leaves
    result = []
    def gather_nodes(v: Vert) -> None:
        if not v.children: # this is a Slurm node
            result.append(v.name)
        for u in v.children.values():
            gather_nodes(u)
    gather_nodes(root)
    return result


class Instance:
    def __init__(self, name: str, zone: str, physical_host: Optional[str]):
        self.name = name
        self.zone = zone
        self.physical_host = physical_host


def make_path(node_name: str, inst: Optional[Instance]) -> List[str]:
    if not inst:  # node with unknown instance (e.g. hybrid cluster)
        return ["unknown", node_name]
    zone = f"zone_{inst.zone}"
    if not inst.physical_host:  # node without physical host info (e.g. no placement policy)
        return [zone, "unknown", node_name]

    assert inst.physical_host.startswith("/"), f"Unexpected physicalHost: {inst.physical_host}"
    parts = inst.physical_host[1:].split("/")
    if len(parts) >= 4:
        return [*parts, node_name]
    return [zone, *parts, node_name]


def to_hostnames(nodelist: str) -> List[str]:
    cmd = ["scontrol", "show", "hostnames", nodelist]
    out = subprocess.run(cmd, check=True, stdout=subprocess.PIPE).stdout
    return [n.decode("utf-8") for n in out.splitlines()]


def get_instances(node_names: List[str]) -> Dict[str, object]:
    fmt = (
        "--format=csv[no-heading,separator=','](zone,resourceStatus.physicalHost,name)"
    )
    cmd = ["gcloud", "compute", "instances", "list", fmt]

    scp = os.path.commonprefix(node_names)
    if scp:
        cmd.append(f"--filter=name~'{scp}.*'")
    out = subprocess.run(cmd, check=True, stdout=subprocess.PIPE).stdout
    d = {}
    for line in out.splitlines():
        zone, physical_host, name = line.decode("utf-8").split(",")
        d[name] = Instance(name, zone, physical_host)
    return {n: d.get(n) for n in node_names}


def main(args) -> None:
    nodelist = args.nodelist or os.getenv("SLURM_NODELIST")
    if not nodelist:
        raise ValueError("nodelist is not provided and SLURM_NODELIST is not set")
    
    if args.ntasks_per_node is None:
        args.ntasks_per_node = int(os.getenv("SLURM_NTASKS_PER_NODE", "") or 1)
    assert args.ntasks_per_node > 0

    output = args.output or f"hosts.{uuid.uuid4()}"

    node_names = to_hostnames(nodelist)
    instannces = get_instances(node_names)
    paths = [make_path(n, instannces[n]) for n in node_names]
    ordered = order(paths)

    with open(output, "w") as f:
        for node in ordered:
            for _ in range(args.ntasks_per_node):
                f.write(node)
                f.write("\n")
    print(output)


if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawTextHelpFormatter)
    parser.add_argument(
        "--nodelist",
        type=str,
        help="Slurm 'hostlist expression' of nodes to sort, if not set the value of SLURM_NODELIST environment variable will be used",
    )
    parser.add_argument(
        "--ntasks-per-node",
        type=int,
        help="""Number of times to repeat each node in resulting sorted list.
If not set, the value of SLURM_NTASKS_PER_NODE environment variable will be used, 
if neither is set, defaults to 1""",
    )
    parser.add_argument(
        "--output", type=str, help="Output file to write, defaults to 'hosts.<uuid>'"
    )
    args = parser.parse_args()
    main(args)
