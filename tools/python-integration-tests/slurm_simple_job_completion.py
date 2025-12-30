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
            state = self.job_state(job_id)
            self.assertIn("COMPLETED", state, f"Job {job_id} did not complete successfully. Final state is {state}.")
            log.info(f"JobID {job_id} finished successfully.")

    def wait_until_squeue_is_empty(self):
        while True:
            stdout = ssh.exec_and_check(self.ssh_client, 'squeue')
            log.debug(f"squeue:\n{stdout}")
            lines = stdout.splitlines()[1:] # Skip header

            if not lines:
                log.info("squeue is empty.")
                break
            log.info(f"Waiting for {len(lines)} jobs to complete...")
            time.sleep(5)

    def job_state(self, job_id: str) -> str:
        """Gets the final state of a job using sacct."""
        cmd = f'sacct -j {job_id} --json --format=JobID,State'
        stdout = ssh.exec_and_check(self.ssh_client, cmd)
        log.debug(f"sacct output for job {job_id}:\n{stdout}")
        try:
            data = json.loads(stdout)
            jobs = data.get("jobs")
            if jobs:
                for job in jobs:
                    # The job_id from sacct can be an int, but the input job_id is a string.
                    # Job steps have suffixes (e.g., '123.batch'), so we look for an exact match
                    # with the main job ID to get the overall job state.
                    if str(job.get("job_id")) == job_id:
                        return job["state"]["current"]

            # This is reached if jobs is empty or the main job ID was not found.
            log.warning(f"No job information found in sacct for JobID: {job_id}")
            return "NOT_FOUND"
        except (IndexError, KeyError, json.JSONDecodeError) as e:
            log.error(f"Error parsing sacct output for job {job_id}: {e}\nOutput: {stdout}")
            return "PARSE_ERROR"

    def submit_job(self, cmd: str) -> str:
        stdout = ssh.exec_and_check(self.ssh_client, cmd)
        return stdout.split()[-1]

if __name__ == "__main__":
    test.slurmtests_main()
