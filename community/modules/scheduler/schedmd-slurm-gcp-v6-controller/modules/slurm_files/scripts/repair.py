#!/usr/bin/env python3

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
    operations = _get_operations()
    return node in operations and operations[node]["status"] == "REPAIR_IN_PROGRESS"

def _get_operations():
    """Get all repair operations from the file."""
    if not REPAIR_FILE.exists():
        return {}
    with open(REPAIR_FILE, 'r', encoding='utf-8') as f:
        try:
            return json.load(f)
        except json.JSONDecodeError:
            log.error(f"Failed to decode JSON from {REPAIR_FILE}, returning empty operations list.")
            return {}

def _write_all_operations(operations):
    """Store the operations to the file safely."""
    try:
        with open(REPAIR_FILE, 'a', encoding='utf-8') as f:
            try:
                fcntl.lockf(f, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except (IOError, BlockingIOError):
                log.warning(f"Could not acquire lock on {REPAIR_FILE}. Another process may be running.")
                return False

            try:
                f.seek(0)
                f.truncate()
                json.dump(operations, f, indent=4)
                f.flush()
                return True
            finally:
                fcntl.lockf(f, fcntl.LOCK_UN)
    except (IOError, TypeError) as e:
        log.error(f"Failed to store repair operations to {REPAIR_FILE}: {e}")
        return False

def store_operation(node, operation_id, reason):
    """Store a single repair operation."""
    operations = _get_operations()
    operations[node] = {
        "operation_id": operation_id,
        "reason": reason,
        "status": "REPAIR_IN_PROGRESS",
        "timestamp": datetime.now(timezone.utc).isoformat(),
    }
    if not _write_all_operations(operations):
        log.error(f"Failed to persist repair operation for node {node}.")

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
        log.info(f"gcloud compute instances report-host-as-faulty stdout: {result.stdout.strip()}")
        op = json.loads(result.stdout)
        if isinstance(op, list):
            op = op[0]
        return op["name"]
    except (subprocess.CalledProcessError, json.JSONDecodeError) as e:
        log.error(f"Failed to call or parse R&R API response for {node}: {e}")
        return None
    except Exception as e:
        log.error(f"An unexpected error occurred while calling R&R API for {node}: {e}")
        return None

def _get_operation_status(operation_id):
    """Get the status of a GCP operation."""
    cmd = f'gcloud compute operations list --filter="name={operation_id}" --format=json'
    try:
        result = run(cmd)
        operations_list = json.loads(result.stdout)
        if operations_list:
            return operations_list[0]

        return None 
    except (subprocess.CalledProcessError, json.JSONDecodeError) as e:
        log.error(f"Failed to get or parse operation status for {operation_id}: {e}")
        return None
    except Exception as e:
        log.error(f"An unexpected error occurred while getting operation status for {operation_id}: {e}")
        return None

def poll_operations():
    """Poll the status of ongoing repair operations."""
    operations = _get_operations()
    if not operations:
        return

    log.info("Polling repair operations")
    for node, op_details in operations.items():
        if op_details["status"] == "REPAIR_IN_PROGRESS":
            gcp_op_status = _get_operation_status(op_details["operation_id"])
            if not gcp_op_status:
                continue

            if gcp_op_status.get("status") == "DONE":
                if gcp_op_status.get("error"):
                    log.error(f"Repair operation for {node} failed: {gcp_op_status['error']}")
                    op_details["status"] = "FAILURE"
                    run(f"{lookup().scontrol} update nodename={node} state=down reason='Repair failed'")
                else:
                    log.info(f"Repair operation for {node} succeeded. Powering down the VM")
                    run(f"{lookup().scontrol} update nodename={node} state=power_down reason='Repair succeeded'")
                    op_details["status"] = "SUCCESS"

    if not _write_all_operations(operations):
        log.error("Failed to persist updated repair operations state after polling.")
