#!/usr/bin/env python3

# Copyright (C) SchedMD LLC.
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

import fcntl
import json
import logging
from datetime import datetime, timezone
from pathlib import Path
import subprocess
import util
from util import run, to_hostlist, lookup
log = logging.getLogger()
REPAIR_FILE = Path("/slurm/repair_operations.json")
REPAIR_REASONS = frozenset(["PERFORMANCE", "SDC", "XID", "unspecified"])

def is_node_being_repaired(node):
    """Check if a node is currently being repaired."""
    operations = get_operations()
    return node in operations and operations[node]["status"] == "REPAIR_IN_PROGRESS"

def get_operations():
    """Get all repair operations from the file."""
    if not REPAIR_FILE.exists():
        return {}
    with open(REPAIR_FILE, 'r', encoding='utf-8') as f:
        try:
            return json.load(f)
        except json.JSONDecodeError:
            return {}

def store_operations(operations):
    """Store the operations to the file."""
    with open(REPAIR_FILE, 'w', encoding='utf-8') as f:
        fcntl.lockf(f, fcntl.LOCK_EX)
        try:
            json.dump(operations, f, indent=4)
        finally:
            fcntl.lockf(f, fcntl.LOCK_UN)

def store_operation(node, operation_id, reason):
    """Store a single repair operation."""
    operations = get_operations()
    operations[node] = {
        "operation_id": operation_id,
        "reason": reason,
        "status": "REPAIR_IN_PROGRESS",
        "timestamp": datetime.now(timezone.utc).isoformat(),
    }
    store_operations(operations)

def call_rr_api(node, reason):
    """Call the R&R API for a given node."""
    log.info(f"Calling R&R API for node {node} with reason {reason}")
    inst = lookup().instance(node)
    if not inst:
        log.error(f"Instance {node} not found, cannot report fault.")
        return None
    cmd = f"gcloud compute instances report-host-as-faulty {node} --async --disruption-schedule=IMMEDIATE --fault-reasons=behavior={reason},description='VM is managed by Slurm' --zone={inst.zone} --format=json"
    try:
        result = run(cmd)
        op = json.loads(result.stdout)
        if isinstance(op, list):
            op = op[0]
        return op["name"]
    except subprocess.CalledProcessError as e:
        log.error(f"Failed to call R&R API for {node} due to command execution error: {e}")
        return None
    except json.JSONDecodeError as e:
        log.error(f"Failed to parse R&R API response for {node} due to JSON decode error: {e}")
        return None
    except Exception as e:
        log.error(f"An unexpected error occurred while calling R&R API for {node}: {e}")
        return None

def get_operation_status(operation_id):
    """Get the status of a GCP operation."""
    cmd = f'gcloud compute operations list --filter="name={operation_id}" --format=json'
    try:
        result = run(cmd)
        operations_list = json.loads(result.stdout)
        if operations_list and len(operations_list) > 0:
            return operations_list[0]

        return None 
    except subprocess.CalledProcessError as e:
        log.error(f"Failed to get operation status for {operation_id} due to command execution error: {e}")
        return None
    except json.JSONDecodeError as e:
        log.error(f"Failed to parse operation status for {operation_id} due to JSON decode error: {e}")
        return None
    except Exception as e:
        log.error(f"An unexpected error occurred while getting operation status for {operation_id}: {e}")
        return None

def poll_operations():
    """Poll the status of ongoing repair operations."""
    operations = get_operations()
    if not operations:
        return

    log.info("Polling repair operations")
    for node, op_details in operations.items():
        if op_details["status"] == "REPAIR_IN_PROGRESS":
            op_status = get_operation_status(op_details["operation_id"])
            if not op_status:
                continue

            if op_status.get("status") == "DONE":
                if op_status.get("error"):
                    log.error(f"Repair operation for {node} failed: {op_status['error']}")
                    op_details["status"] = "FAILURE"
                    run(f"{lookup().scontrol} update nodename={node} state=down reason='Repair failed'")
                else:
                    log.info(f"Repair operation for {node} succeeded. Powering down the VM")
                    run(f"{lookup().scontrol} update nodename={node} state=power_down reason='Repair succeeded'")
                    op_details["status"] = "SUCCESS"
        elif op_details["status"] == "SUCCESS":
            inst = lookup().instance(node)
            if inst and inst.status == "RUNNING":
                log.info(f"Node {node} is back online.")
                op_details["status"] = "RECOVERED"

    store_operations(operations)
