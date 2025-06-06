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

from deployment import Deployment
from test import SlurmTest
import sys
import unittest
import time
import json
import ssh

import logging
log = logging.getLogger()

class SlurmSimpleJobCompletionTest(SlurmTest):
    # Class to test simple slurm job completion
    def __init__(self, unused_deployment):
        super().__init__(Deployment(self.Blueprint))
        self.job_list = {}

    def runTest(self):
        # Submits 5 jobs and checks if they are successful.
        for i in range(5):
            self.submit_job('sbatch -N 1 --wrap "sleep 20"')
        self.monitor_squeue()

        for job_id in self.job_list.keys():
            result = self.is_job_complete(job_id)
            self.assertTrue(result, f"Something went wrong with job:{job_id}.")
            log.info(f"JobID {job_id} finished successfully.")

    def monitor_squeue(self):
        # Monitors squeue and updates self.job_list until all running jobs are complete.
        lines = []

        while True:
            stdout = ssh.exec_and_check(self.ssh_login(), 'squeue')
            lines = stdout.splitlines()[1:] # Skip header

            if not lines:
                break
            for line in lines:
                log.info(f"squeue: {line}")
                job_id, partition, _, _, state, times, nodes, nodelist = line.split()

                if job_id not in self.job_list:
                    log.warning(f"Job id {job_id} is not recognized.")
                else:
                    self.job_list[job_id].update({
                        "partition": partition,
                        "state": state,
                        "time": times,
                        "nodes": nodes,
                        "nodelist": nodelist,
                    })
            time.sleep(5)

    def is_job_complete(self, job_id: str) -> bool:
        # Checks if a job successfully completed.
        stdout = ssh.exec_and_check(self.ssh_login(), f"scontrol show job {job_id} --json")
        content = json.loads(stdout)
        log.info(f"show job {job_id}: {content}")
        return content["jobs"][0]["job_state"][0] == "COMPLETED"

    def submit_job(self, cmd: str) -> None:
        stdout = ssh.exec_and_check(self.ssh_login(), cmd)
        job_id = stdout.split()[-1]
        self.job_list[job_id] = {}

if __name__ == "__main__":
    logging.basicConfig(level=logging.DEBUG)
    if len(sys.argv) > 1:
        SlurmSimpleJobCompletionTest.Blueprint = sys.argv.pop()
    unittest.main()
