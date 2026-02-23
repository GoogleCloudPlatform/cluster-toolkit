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
import test
import time

class SlurmReconfigureSize(test.SlurmTest):
    INIT_BP = "tools/python-integration-tests/blueprints/slurm-reconfig-before.yaml"
    RECONFIG_BP = "tools/python-integration-tests/blueprints/slurm-reconfig-after.yaml"
    
    def get_deployment(self) -> Deployment:
        return Deployment(SlurmReconfigureSize.INIT_BP)
    
    
    def runTest(self):
        # Check 5 nodes are available
        self.assert_equal(len(self.get_nodes()), 5)
        
        self.deployment = Deployment(SlurmReconfigureSize.RECONFIG_BP)
        self.deployment.deploy()
        
        # Wait 90 seconds for reconfig
        time.sleep(90)

        # Check 3 nodes are available
        self.assert_equal(len(self.get_nodes()), 3)

if __name__ == "__main__":
    test.slurmtests_main()
