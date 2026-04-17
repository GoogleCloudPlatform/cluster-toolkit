#!/usr/bin/env python3

# Copyright 2024 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import json
import logging
import os
import re
import subprocess
from datetime import datetime
from subprocess import PIPE

# Path to the file storing repair operations
OPERATIONS_FILE = "/var/log/slurm/repair_operations.json"


def is_node_being_repaired(node_name):
    """Check if a node is currently being repaired."""
    operations = get_operations()
    return any(op["node_name"] == node_name for op in operations)


def get_operations():
    """Retrieve the list of repair operations from the file."""
    if not os.path.exists(OPERATIONS_FILE):
        return []
    with open(OPERATIONS_FILE) as f:
        return json.load(f)


def store_operations(operations):
    """Store the list of repair operations to the file."""
    with open(OPERATIONS_FILE, "w") as f:
        json.dump(operations, f)


def store_operation(operation):
    """Append a repair operation to the file."""
    operations = get_operations()
    operations.append(operation)
    store_operations(operations)


def call_rr_api(node_name):
    """Initiate a repair operation for a node and return the operation details."""
    logging.info(f"Calling R&R API for node: {node_name}")
    gcloud_command = (
        "gcloud compute instances report-host-as-faulty"
        f" {node_name} --zone $(curl -sS -H 'Metadata-Flavor: Google'"
        " http://metadata.google.internal/computeMetadata/v1/instance/zone |"
        " cut -d'/' -f4) --project $(curl -sS -H 'Metadata-Flavor: Google'"
        " http://metadata.google.internal/computeMetadata/v1/project/project-id)"
        " --format=json"
    )
    result = subprocess.run(gcloud_command, shell=True, check=True, stdout=PIPE)
    operation = json.loads(result.stdout)
    operation["node_name"] = node_name
    operation["timestamp"] = datetime.utcnow().isoformat()
    store_operation(operation)
    return operation


def get_operation_status(operation_name, zone, project):
    """Get the status of a GCP operation."""
    gcloud_command = (
        f"gcloud compute zone-operations describe {operation_name} --zone {zone}"
        f" --project {project} --format=json"
    )
    result = subprocess.run(gcloud_command, shell=True, check=True, stdout=PIPE)
    return json.loads(result.stdout)


def poll_operations():
    """Poll the status of ongoing repair operations and update node states."""
    operations = get_operations()
    if not operations:
        return

    logging.info(f"Polling {len(operations)} repair operations")
    remaining_operations = []
    for op in operations:
        project = op["project"]
        zone = op["zone"].split("/")[-1]
        status = get_operation_status(op["name"], zone, project)
        if status["status"] == "DONE":
            logging.info(f"Operation for node {op['node_name']} is DONE")
            # Set node state back to IDLE
            subprocess.run(
                f"scontrol update nodename={op['node_name']} state=IDLE", shell=True
            )
        else:
            remaining_operations.append(op)
    store_operations(remaining_operations)

