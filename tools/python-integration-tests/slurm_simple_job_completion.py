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

import test
import time
import json
import logging
import ssh

log = logging.getLogger()


class SlurmSimpleJobCompletionTest(test.SlurmTest):
    # Class to test simple slurm job completion
    def runTest(self):
        # Submits 5 jobs and checks if they are successful.
        job_ids = []
        for _ in range(5):
            job_ids.append(self.submit_job('sbatch -N 1 --wrap "sleep 20"'))
        self.wait_until_squeue_is_empty()

        for job_id in job_ids:
            self.assertIn("COMPLETED", self.job_state(job_id), f"Something went wrong with JobID:{job_id}.")
            print(f"JobID {job_id} finished successfully.")

    def wait_until_squeue_is_empty(self):
        while True:
            stdout = ssh.exec_and_check(self.ssh_client, 'squeue')
            log.debug(f"squeue:\n{stdout}")
            lines = stdout.splitlines()[1:] # Skip header

            if not lines:
                break
            time.sleep(5)

    def job_state(self, job_id: str) -> list[str]:
        stdout = ssh.exec_and_check(self.ssh_client, f'scontrol show job {job_id} --json')
        return json.loads(stdout)["jobs"][0]["job_state"]

    def submit_job(self, cmd: str) -> str:
        stdout = ssh.exec_and_check(self.ssh_client, cmd)
        return stdout.split()[-1]

if __name__ == "__main__":
    test.slurmtests_main()
