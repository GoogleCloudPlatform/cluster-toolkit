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

import logging
from pathlib import Path
import util
from util import run, to_hostlist, lookup

log = logging.getLogger()

REPAIR_FILE = Path(f"{util.dirs.state}/repair_operations.json")
REPAIR_REASONS = frozenset(["PERFORMANCE", "SDC", "XID", "unspecified"])

import json
import fcntl
from datetime import datetime, timezone

def is_node_being_repaired(node):
    """Check if a node is currently being repaired."""
    operations = get_operations()
    return node in operations and operations[node]["status"] == "REPAIR_IN_PROGRESS"

def get_operations():
    """Get all repair operations from the file."""
    if not REPAIR_FILE.exists():
        return {}
    with open(REPAIR_FILE, 'r') as f:
        try:
            return json.load(f)
        except json.JSONDecodeError:
            return {}

def store_operations(operations):
    """Store the operations to the file."""
    with open(REPAIR_FILE, 'w') as f:
        fcntl.lockf(f, fcntl.LOCK_EX)
        try:
            json.dump(operations, f, indent=4)
        finally:
            fcntl.lockf(f, fcntl.LOCK_UN)

def call_rr_api(nodes, reason):
    """Call the R&R API for the given nodes."""
    log.info(f"Calling R&R API for nodes {to_hostlist(nodes)} with reason {reason}")
    for node in nodes:
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
        except Exception as e:
            log.error(f"Failed to call R&R API for {node}: {e}")
            return None

def get_operation_status(operation_id):
    """Get the status of a GCP operation."""
    cmd = f"gcloud compute operations describe {operation_id} --format=json"
    try:
        result = run(cmd)
        return json.loads(result.stdout)
    except Exception as e:
        log.error(f"Failed to get operation status for {operation_id}: {e}")
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
                    log.info(f"Repair operation for {node} succeeded.")
                    op_details["status"] = "SUCCESS"
                    inst = lookup().instance(node)
                    if inst and not inst.scheduling.automatic_restart:
                        log.info(f"Manually restarting node {node}.")
                        try:
                            run(f"gcloud compute instances start {node} --zone={inst.zone}")
                        except Exception as e:
                            log.error(f"Failed to restart instance {node}: {e}")
                            run(f"{lookup().scontrol} update nodename={node} state=down reason='Repair successful, but restart failed'")
        
        if op_details["status"] == "SUCCESS":
            inst = lookup().instance(node)
            if inst and inst.status == "RUNNING":
                log.info(f"Node {node} is back online, undraining.")
                undrain_node(node)
                op_details["status"] = "RECOVERED"

    store_operations(operations)
def undrain_node(node):
    """Undrain a node."""
    log.info(f"Undraining node {node}")
    run(f"{lookup().scontrol} update nodename={node} state=resume")
