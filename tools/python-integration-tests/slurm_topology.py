# Copyright 2026 "Google LLC"
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

from deployment import Deployment
from collections import defaultdict
import logging
import time
import test

logging.basicConfig(level=logging.INFO) 
log = logging.getLogger()

class SlurmTopologyTest(test.SlurmTest):
    # Class to test Slurm topology
    def get_deployment(self) -> Deployment:
        return Deployment("tools/python-integration-tests/blueprints/topology-test.yaml")

    def runTest(self):
        # Checks isomorphism of last layer of nodes to determine topology.
        r_rack, s_rack = defaultdict(set), defaultdict(set)
        nodes = self.get_nodes()

        self.get_slurm_topology()
        for node in nodes:
            r_rack[self.get_real_rack(node)].add(node)
            s_rack[self.get_slurm_rack(node)].add(node)

        r_rack_set = [set(v) for v in r_rack.values()]
        s_rack_set = [set(v) for v in s_rack.values()]

        self.assert_equal({frozenset(s) for s in r_rack_set}, {frozenset(s) for s in s_rack_set}, "The two sets did not match.")

    def get_slurm_topology(self):
        stdin, stdout, stderr = self.ssh_client.exec_command("scontrol show topo")
        log.info(f"Slurm topology: {stdout.read().decode()}")

    def get_node_depth(self, switch_name: str):
        return switch_name.count("_")
    
    def get_real_rack(self, node: str):
        max_retries = 15
        wait_seconds = 20
        for attempt in range(max_retries):
            try:
                result = self.run_command(f"gcloud compute instances describe {node} --zone={self.deployment.zone} --project={self.deployment.project_id} --format='value(resourceStatus.physicalHost)'")
                physicalHost = result.stdout.strip()
                if physicalHost:
                    log.info(f"physicalHost for {node}: {physicalHost}")
                    parts = [p for p in physicalHost.split("/") if p]
                    if len(parts) > 1:
                        return f"{parts[0]}/{parts[1]}"
                    elif len(parts) > 0:
                        return parts[0]
                else:
                    log.info(f"Attempt {attempt + 1}: physicalHost for {node} is currently empty. Retrying...")
            except Exception as e:
                log.info(f"Attempt {attempt + 1}: Failed to fetch physicalHost for {node} ({e}). Retrying in {wait_seconds}s...")
            time.sleep(wait_seconds)
        
        # Fallback to node name or a predictable segment if physicalHost remains unavailable
        log.warning(f"Could not retrieve physicalHost for {node} after {max_retries} attempts. Falling back to default rack identifier.")
        return node.rsplit("-", 1)[0]

    def get_slurm_rack(self, node: str):
        max_retries = 5     
        wait_seconds = 30    
        depth = -1
        last_err = ""
        switch_name = ""

        for attempt in range(max_retries):
            stdin, stdout, stderr = self.ssh_client.exec_command(f"scontrol show topology node={node} | tail -1 | cut -d' ' -f1")
            switch_name = stdout.read().decode().strip()
            last_err = stderr.read().decode()
            
            if last_err:
                log.info(f"Attempt {attempt + 1}: Slurm error for {node} ({last_err.strip()}). Retrying...")
                time.sleep(wait_seconds)
                continue

            depth = self.get_node_depth(switch_name)
            
            if depth == 2:
                log.info(f"Slurm rack for {node} ready: {switch_name}")
                return switch_name
                
            log.info(f"Attempt {attempt + 1}: Topology depth for {node} is {depth}. Waiting {wait_seconds}s for Slurm to update...")
            time.sleep(wait_seconds)
            
        if last_err:
            raise Exception(f"Slurm topology error for {node} after {max_retries} attempts: {last_err}")
        self.assert_equal(depth, 2, f"{node} has a topology depth of {depth} which does not match expected topology depth of 2.")
        return switch_name

if __name__ == "__main__":
    test.slurmtests_main()
