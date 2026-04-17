#!/usr/bin/env python3

# Copyright 2023 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
#
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import logging
import re
import subprocess
from dataclasses import dataclass
from subprocess import PIPE

LOG_FILE = "/var/log/slurm/gcp_sync.log"
# logging.basicConfig(filename=LOG_FILE, level=logging.INFO)
logging.basicConfig(level=logging.INFO)


@dataclass
class SlurmNode:
    name: str
    state: str
    reason: str


def get_slurm_nodes():
    """Get the state of all nodes in the cluster."""
    scontrol_cmd = "scontrol show node"
    result = subprocess.run(scontrol_cmd, shell=True, check=True, stdout=PIPE)
    output = result.stdout.decode()
    nodes = {}
    for node_output in output.split("\n\n"):
        node = parse_scontrol_output(node_output)
        if node:
            nodes[node.name] = node
    return nodes


def parse_scontrol_output(output):
    """Parse the output of scontrol show node."""
    m = re.search(r"NodeName=(\\S+)", output)
    if not m:
        return None
    name = m.group(1)
    m = re.search(r"State=(\\S+)", output)
    state = m.group(1) if m else ""
    m = re.search(r"Reason=(.*)", output)
    reason = m.group(1).strip() if m else ""
    return SlurmNode(name, state, reason)


def main():
    """main function."""
    logging.info("Starting slurmsync")
    nodes = get_slurm_nodes()
    logging.info(f"Found {len(nodes)} nodes")


if __name__ == "__main__":
    main()
