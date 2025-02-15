
#!/usr/bin/env python
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

#filename: topological_pytorch.py
import os
import torch
import torch.distributed as dist
import socket
import subprocess
import argparse

parser = argparse.ArgumentParser()
parser.add_argument("--topology", action=argparse.BooleanOptionalAction)
args = parser.parse_args()

hostname = socket.getfqdn()
if args.topology:
    # These are populated by Slurm
    local_rank = int(os.environ["SLURM_LOCALID"])
    global_rank = int(os.environ["SLURM_PROCID"])
    world_size = int(os.environ["SLURM_NPROCS"])
    procs_per_node = int(os.environ["SLURM_NTASKS_PER_NODE"])

    # Must set rank and world_size based on SLURM_PROCID and SLURM_NPROCS
    dist.init_process_group("nccl", rank=global_rank, world_size=world_size)
else:
    # These are populated by torchrun
    local_rank = int(os.environ["LOCAL_RANK"])
    global_rank = int(os.environ["RANK"])
    world_size = int(os.environ["WORLD_SIZE"])
    procs_per_node = int(os.environ["LOCAL_WORLD_SIZE"])

    # Torchrun handles rank allocation
    dist.init_process_group("nccl")

# Must attach device based on the local rank.
torch.cuda.set_device(local_rank)

# Get the physical host for the current task to print later
physical_host = subprocess.check_output([
    "curl", "-s",
    "http://metadata.google.internal/computeMetadata/v1/instance/attributes/physical_host",
    "-H", "Metadata-Flavor: Google"
]).decode('utf-8')

# Create an output to collect from the all-gather
output = [None for _ in range(world_size)]
dist.all_gather_object(output, [global_rank, hostname, physical_host])
if global_rank == 0:
    # Print out ordered set of hostnames from all-gather
    print("rank\thostname\tphysical_host")
    # Skip to print every procs_per_node to keep output compact
    for result in output[::procs_per_node]:
        print("\t".join(map(str,result)))

dist.destroy_process_group()
