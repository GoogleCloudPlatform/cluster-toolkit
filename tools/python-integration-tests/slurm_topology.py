# Copyright 2024 "Google LLC"
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

from ssh import SSHManager
from deployment import Deployment
from test import SlurmTest
from collections import defaultdict
import unittest
import logging

class SlurmTopologyTest(SlurmTest):
    # Class to test Slurm topology
    def __init__(self, deployment):
        super().__init__(Deployment("tools/python-integration-tests/blueprints/topology-test.yaml"))

    def runTest(self):
        # Checks isomorphism of last layer of nodes to determine topology.
        r_rack, s_rack = defaultdict(set), defaultdict(set)
        nodes = self.get_nodes()

        for node in nodes:
            r_rack[self.get_real_rack(node)].add(node)
            s_rack[self.get_slurm_rack(node)].add(node)

        r_rack_set = [set(v) for v in r_rack.values()]
        s_rack_set = [set(v) for v in s_rack.values()]

        self.assert_equal(r_rack_set, s_rack_set, "The two sets did not match.")

    def get_slurm_topology(self):
        stdin, stdout, stderr = self.ssh_client.exec_command("scontrol show topo")
        return stdout.read().decode() 

    def get_node_depth(self, switch_name: str):
        return switch_name.count("_")

    def get_real_rack(self, node: str):
        result = self.run_command(f"gcloud compute instances describe {node} --zone={self.deployment.zone} --project={self.deployment.project_id} --format='value(resourceStatus.physicalHost)'")
        return result.stdout.split("/")[1]

    def get_slurm_rack(self, node: str):
        stdin, stdout, stderr = self.ssh_client.exec_command(f"scontrol show topology {node} | tail -1 | cut -d' ' -f1")
        switch_name = stdout.read().decode()
        self.assert_equal(self.get_node_depth(switch_name), 2, f"{node} does not have the expected topology depth of 2."),
        return switch_name

if __name__ == "__main__":
    unittest.main()
