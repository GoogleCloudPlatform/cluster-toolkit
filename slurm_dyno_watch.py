#!/bin/bash python3
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


import subprocess
from dataclasses import dataclass
from typing import List
import time

@dataclass
class Job:
    id: int
    partition: str

"""
# Fake node node that only purpose is to be a target for feature addition
NodeName=feature_aggregator State=UNKNOWN

# Dynamic nodeset that would contain nodes from MIGs
Nodeset=ondemand

# Empty nodeset with only puprpose to make `ondemand` parition "schedulable"
NodeSet=ondemand-empty

# Partition to send `srun` against
PartitionName=ondemand Nodes=ondemand-empty State=UP Default=NO

# Partition where jobs from `ondemand` partition will be migrated to
PartitionName=ondemand-exec Nodes=ondemand State=UP Default=NO
"""
# map of "intake" parition to "exec" partition
PARTITIONS = {
    "ondemand": "ondemand-exec"
}

def get_parition_jobs(partition: str) -> List[Job]:
    result = subprocess.run(["squeue", "-p", partition, "-h", "-o", "%i"], capture_output=True)
    job_ids = result.stdout.decode().split("\n")
    return [Job(id=int(job_id), partition=partition) for job_id in job_ids if job_id]

def migrate_job(job: Job, exec_partition: str) -> None:
    print(f"Migrating job {job.id} from {job.partition} to {exec_partition}") 
    
    feature = f"exclusive-job-{job.id}"
    # Add unique feature to the Slurm, to be remove once Slurm supports unknown features
    fake_node = "feature_aggregator"
    subprocess.run(["scontrol", "update", f"NodeName={fake_node}", f"AvailableFeatures={feature}"])

    subprocess.run(["scontrol", "update", f"JobId={job.id}", f"Partition={exec_partition}", f"Features={feature}"])
    # TODO: proceed with launching MIG in parition `exec_partition` and feature `feature`

def loop() -> List[Job]:
    for intake, exec in PARTITIONS.items():
        for job in get_parition_jobs(intake):
            migrate_job(job, exec)


if __name__ == "__main__":
    while True:
        loop()
        time.sleep(3)
