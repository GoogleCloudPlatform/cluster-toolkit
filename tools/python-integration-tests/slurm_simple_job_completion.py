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
import unittest
import time
import json

class SlurmSimpleJobCompletionTest(SlurmTest):
    # Class to test simple slurm job completion
    def __init__(self, deployment):
        super().__init__(Deployment("tools/python-integration-tests/blueprints/slurm-simple.yaml"))
        self.job_list = {}

    def runTest(self):
        # Submits 5 jobs and checks if they are successful.
        for i in range(5):
            self.submit_job('sbatch -N 1 --wrap "sleep 20"')
        self.monitor_squeue()

        for job_id in self.job_list.keys():
            result = self.is_job_complete(job_id)
            self.assert_equal(True, result, f"Something went wrong with JobID:{job_id}.")
            print(f"JobID {job_id} finished successfully.")

    def monitor_squeue(self):
        # Monitors squeue and updates self.job_list until all running jobs are complete.
        lines = []

        while True:
            stdin, stdout, stderr = self.ssh_client.exec_command('squeue')

            lines = stdout.read().decode().splitlines()[1:] # Skip header

            if not lines:
                break
            for line in lines:
                parts = line.split()
                job_id, partition, _, _, state, times, nodes, nodelist = line.split()

                if job_id not in self.job_list:
                    print(f"Job id {job_id} is not recognized.")
                else:
                    self.job_list[job_id].update({
                        "partition": partition,
                        "state": state,
                        "time": times,
                        "nodes": nodes,
                        "nodelist": nodelist,
                    })
            time.sleep(5)

    def is_job_complete(self, job_id: str):
        # Checks if a job successfully completed.
        stdin, stdout, stderr = self.ssh_client.exec_command(f'scontrol show job {job_id} --json')
        content = json.load(stdout)
        return content["jobs"][0]["job_state"][0] == "COMPLETED"

    def submit_job(self, cmd: str):
        stdin, stdout, stderr = self.ssh_client.exec_command(cmd)
        jobID = stdout.read().decode().split()[-1]
        self.job_list[jobID] = {}

if __name__ == "__main__":
    unittest.main()
