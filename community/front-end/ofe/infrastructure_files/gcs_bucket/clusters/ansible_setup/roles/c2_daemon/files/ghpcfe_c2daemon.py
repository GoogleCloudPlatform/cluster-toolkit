#!/usr/bin/env python3
# Copyright 2022 Google LLC
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

"""Cluster management daemon for the Google Cluster Toolkit Frontend"""

import grp
import json
import logging.handlers
import os
import pwd
import shutil
import socket
import subprocess
import sys
import time
import concurrent.futures
from functools import wraps
from pathlib import Path
from urllib.parse import urlparse
from threading import Thread
import traceback

import pexpect
import requests
import yaml
from google.cloud import pubsub
from google.cloud import storage as gcs

## N.B In almost all cases we can do nothing about failures other than report
## back to the frontend, so there is no mileage in more detailed handling
# pylint: disable=broad-except

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)
# Send to syslog somehow
logger.addHandler(logging.handlers.SysLogHandler(address="/dev/log"))

# Set this to non-zero from a callback to cause us to exit
EXIT_CODE = 0

# GCS metadata access
GCS_METADATA_BASEURL = "http://metadata.google.internal/computeMetadata/v1/"
GCS_METADATA_HEADERS = {"Metadata-Flavor": "Google"}

# Caching of oslogin users
_OSLOGIN_CACHE = {}

# Set the env var for testing
with open(
    os.environ.get("GHPCFE_CFG", "/usr/local/etc/ghpcfe_c2.yaml"),
    "r",
    encoding="utf-8",
) as cfg:
    config = yaml.safe_load(cfg)

source_id = f"cluster_{config['cluster_id']}"
cluster_bucket = config["cluster_bucket"]
spack_path = config.get("spack_path", "/opt/cluster/spack")
spack_bin = f"{spack_path}/bin/spack"

pubClient = pubsub.PublisherClient()
subscriber = pubsub.SubscriberClient()
thread_pool = concurrent.futures.ThreadPoolExecutor()

_c2_ackMap = {}

# SLURM command paths - try to find them in common locations
SLURM_PATHS = [
    "/usr/local/bin",
    "/usr/bin",
    "/opt/slurm/bin",
    "/usr/local/slurm/bin"
]

def find_slurm_command(cmd):
    """Find the full path to a SLURM command"""
    for path in SLURM_PATHS:
        full_path = os.path.join(path, cmd)
        if os.path.exists(full_path) and os.access(full_path, os.X_OK):
            return full_path

    # If not found in common paths, try using PATH
    try:
        result = subprocess.run(["which", cmd], capture_output=True, text=True, check=True)
        return result.stdout.strip()
    except subprocess.CalledProcessError:
        logger.error(f"Could not find {cmd} command")
        return None

# Find SLURM commands
SQUEUE_CMD = find_slurm_command("squeue")
SINFO_CMD = find_slurm_command("sinfo")
SACCT_CMD = find_slurm_command("sacct")
SCONTROL_CMD = find_slurm_command("scontrol")

if not all([SQUEUE_CMD, SINFO_CMD, SACCT_CMD, SCONTROL_CMD]):
    logger.error("Could not find all required SLURM commands. Found:")
    logger.error(f"  squeue: {SQUEUE_CMD}")
    logger.error(f"  sinfo: {SINFO_CMD}")
    logger.error(f"  sacct: {SACCT_CMD}")
    logger.error(f"  scontrol: {SCONTROL_CMD}")
    logger.error("Available SLURM commands in PATH:")
    for path in SLURM_PATHS:
        if os.path.exists(path):
            for file in os.listdir(path):
                if file.startswith('s'):
                    logger.error(f"  {path}/{file}")


def send_message(command, message, extra_attrs=None):
    """Send message to frontend via pubsub"""

    extra_attrs = extra_attrs if extra_attrs else {}
    # We always want our ID in the message
    message["cluster_id"] = config["cluster_id"]
    pubClient.publish(
        config["topic_path"],
        bytes(json.dumps(message), "utf-8"),
        command=command,
        source=source_id,
        **extra_attrs,
    )


def cb_in_thread(func):
    """Decorator wrapper to run callbacks in async threads"""

    @wraps(func)
    def wrapper(*args, **kwargs):
        logger.debug("Spawning Callback in threadpool")
        fut = thread_pool.submit(func, *args, **kwargs)
        try:
            logger.warning(
                "Job had quick exception", exc_info=fut.exception(timeout=0.5)
            )
        except concurrent.futures.TimeoutError:
            # Job is still going - good.
            pass

    return wrapper


def _download_gcs_directory(blob_path: str, target_dir: Path) -> None:
    client = gcs.Client()
    gcs_bucket = client.bucket(cluster_bucket)
    for blob in client.list_blobs(gcs_bucket, prefix=blob_path):
        local_filename = target_dir / blob.name[len(blob_path) + 1 :]
        logger.debug(
            "Attempting to download %s from %s to %s",
            blob.name,
            cluster_bucket,
            local_filename.as_posix(),
        )
        local_filename.parent.mkdir(parents=True, exist_ok=True)
        blob.download_to_filename(local_filename.as_posix())


def _rerun_ansible():
    # Download ansible repo from GCS  (Can't just point at it)
    _download_gcs_directory(
        "clusters/ansible_setup", Path("/tmp/ansible_setup")
    )

    logger.info("Downloaded Ansible Repo.  Beginning playbook")
    try:
        with open("/tmp/ansible_setup/hosts", "w", encoding="utf-8") as fileh:
            fileh.write(f"{socket.gethostname()}\n")

        with open("/tmp/ansible.log", "w", encoding="utf-8") as ansible_log:
            subprocess.run(
                ["ansible-playbook", "./controller.yaml"],
                check=True,
                cwd="/tmp/ansible_setup",
                stdout=ansible_log,
                stderr=subprocess.STDOUT,
            )
    except Exception as err:
        logger.error("Ansible threw an error", exc_info=err)
        raise
    finally:
        logger.info("Uploading ansible log file")
        _upload_log_files(
            {"controller_logs/tmp/ansible.log": "/tmp/ansible.log"}
        )


# Action functions


# @cb_in_thread
def cb_sync(message):
    """Callback for handling cluster syncs"""

    logger.info("Starting sync:  Message: %s", message)
    ackid = message.get("ackid", None)
    response = {"ackid": ackid}
    # Syncing we do these days....
    # Upload latest copies of system logs
    try:
        log_files = [
            "/tmp/setup.log",
            "/var/log/messages",
            "/var/log/slurm/slurmctld.log",
            "/var/log/slurm/resume.log",
            "/var/log/slurm/suspend.log",
        ]
        _upload_log_files({f"controller_logs{f}": f for f in log_files})
    except Exception as err:
        logger.error("Failed to upload log files", exc_info=err)
        response["message"] = str(err)

    # Download & run latest ansible config
    try:
        response["status"] = "e"  # Suggest we're in an error'd state
        _rerun_ansible()
        response["status"] = "r"  # Suggest we're in an error'd state
    except Exception as err:
        logger.error("Failed to download & run ansible", exc_info=err)
        response["message"] = str(err)
    else:
        # Restart Daemon if ansible run was successful
        logger.info("Sending ACK and Attempting to restart c2 daemon")
        response["status"] = "i"
        global EXIT_CODE
        EXIT_CODE = 123  # Magic code for systemctl to restart us
    finally:
        send_message("ACK", response)


def _upload_log_blobs(log_dict):
    client = gcs.Client()
    gcs_bucket = client.bucket(cluster_bucket)
    for path, data in log_dict.items():
        if data:
            continue

        # cluster_bucket is bucket and path...
        full_path = f"clusters/{config['cluster_id']}/{path}"
        blob = gcs_bucket.blob(full_path)
        blob.upload_from_string(data)
    client.close()


def _upload_log_files(log_dict):
    client = gcs.Client()
    gcs_bucket = client.bucket(cluster_bucket)
    for path, filename in log_dict.items():
        if not Path(filename).exists():
            continue

        # cluster_bucket is bucket and path...
        full_path = f"clusters/{config['cluster_id']}/{path}"
        blob = gcs_bucket.blob(full_path)
        blob.upload_from_filename(filename)
    client.close()


def get_slurm_job_state(job_id):
    """Get SLURM job state using the most reliable method available"""
    # First try scontrol for current jobs
    job_state, _, _ = get_individual_job_status(job_id)
    if job_state:
        logger.debug(f"Got job state for {job_id} from scontrol: {job_state}")
        return job_state

    # Fallback to squeue for jobs that might not be in scontrol
    try:
        proc = subprocess.run(
            ["squeue", "--json"], check=True, stdout=subprocess.PIPE
        )
        output = json.loads(proc.stdout)
        for job in output["jobs"]:
            if job["job_id"] == job_id:
                job_state = job.get("job_state")
                if job_state and isinstance(job_state, list) and job_state:
                    logger.debug(f"Got job state for {job_id} from squeue: {job_state[0]}")
                    return job_state[0]
                elif job_state:
                    logger.debug(f"Got job state for {job_id} from squeue: {job_state}")
                    return job_state
        logger.debug(f"Job {job_id} not found in squeue")
        return None
    except Exception as err:
        logger.error("Failed to get job state from squeue for %s: %s", job_id, err)
        return None


def get_slurm_job_info(job_id):
    """Get comprehensive SLURM job information using the most reliable method available"""
    # First try scontrol for detailed information
    job_state, exit_code, user_name = get_individual_job_status(job_id)
    if job_state:
        # Get additional info from squeue if available
        try:
            proc = subprocess.run(
                ["squeue", "--json"], check=True, stdout=subprocess.PIPE
            )
            output = json.loads(proc.stdout)
            for job in output["jobs"]:
                if job["job_id"] == job_id:
                    # Enhance with scontrol data
                    job["job_state"] = job_state
                    if exit_code:
                        job["exit_code"] = exit_code
                    if user_name:
                        job["user_name"] = user_name
                    logger.debug(f"Got comprehensive job info for {job_id}")
                return job
        except Exception as err:
            logger.debug(f"Failed to get additional job info from squeue for {job_id}: {err}")

        # Return basic info from scontrol if squeue fails
        return {
            "job_id": job_id,
            "job_state": job_state,
            "exit_code": exit_code,
            "user_name": user_name
        }

    # Fallback to squeue only
    try:
        proc = subprocess.run(
            ["squeue", "--json"], check=True, stdout=subprocess.PIPE
        )
        output = json.loads(proc.stdout)
        for job in output["jobs"]:
            if job["job_id"] == job_id:
                logger.debug(f"Got job info for {job_id} from squeue")
                return job
        logger.debug(f"Job {job_id} not found in squeue")
        return None
    except Exception as err:
        logger.error("Failed to get job info from squeue for %s: %s", job_id, err)
        return None


def get_recent_job_history():
    """Get recent job history using sacct command"""
    if not SACCT_CMD:
        logger.error("sacct command not found, cannot get job history")
        return None

    try:
        # Get jobs from the last 7 days
        # Try different time formats that work with different SLURM versions
        time_formats = [
            "now-7days",
            "7days",
            "now-1week",
            "now-24hours",
            "24hours",
            "yesterday",
            "now-1day"
        ]

        for time_format in time_formats:
            try:
                result = subprocess.run(
                    [SACCT_CMD, "--json", "--starttime", time_format],
                    capture_output=True,
                    text=True,
                    check=True,
                    timeout=30
                )
                return json.loads(result.stdout)
            except subprocess.CalledProcessError:
                logger.debug("Time format '%s' not supported, trying next", time_format)
                continue
            except subprocess.TimeoutExpired:
                logger.warning("sacct command timed out with format '%s'", time_format)
                continue

        # If all time formats fail, try without time limit (get all recent jobs)
        logger.warning("All time formats failed, trying sacct without time limit")
        result = subprocess.run(
            [SACCT_CMD, "--json"],
            capture_output=True,
            text=True,
            check=True,
            timeout=30
        )
        return json.loads(result.stdout)

    except subprocess.CalledProcessError as e:
        logger.error("Failed to get job history: %s", e)
        return None
    except json.JSONDecodeError as e:
        logger.error("Failed to parse job history: %s", e)
        return None
    except Exception as e:
        logger.error("Unexpected error getting job history: %s", e)
        return None


def process_queue_data(queue_data, node_data):
    """Process queue and node data to extract statistics"""
    if not queue_data or not node_data:
        return []

    # Debug: Log what we're processing
    jobs = queue_data.get("jobs", [])
    nodes = node_data.get("nodes", [])
    logger.debug(f"process_queue_data: Processing {len(jobs)} jobs and {len(nodes)} nodes")

    # Log partition information from SLURM data
    job_partitions = set()
    node_partitions = set()

    for job in jobs:
        job_id = job.get("job_id", "unknown")
        state = job.get("job_state", "UNKNOWN")
        partition = job.get("partition")
        logger.debug(f"process_queue_data: Job {job_id} state = {state}, partition = {partition}")
        if partition:
            job_partitions.add(partition)

    for node in nodes:
        partition = node.get("partition")
        state = node.get("state", "UNKNOWN")
        logger.debug(f"process_queue_data: Node partition = {partition}, state = {state}")
        if partition:
            node_partitions.add(partition)

    logger.debug(f"process_queue_data: Found partitions - jobs: {job_partitions}, nodes: {node_partitions}")

    # Helper to check if a job's state matches a target state (handles list or str)
    def _job_state_matches(state, target):
        if isinstance(state, list):
            return target in state
        return state == target

    # Initialize partition stats - start with empty counts
    partition_stats = {}

    # Get all partitions from node data to ensure we have entries for all partitions
    for node in node_data.get("nodes", []):
        partition = node.get("partition")
        if partition:  # Only add if partition is actually specified
            if partition not in partition_stats:
                partition_stats[partition] = {
                    "pending": 0,
                    "running": 0,
                    "completed": 0
                }

    # If no partitions found from node data, get them from SLURM configuration
    if not partition_stats:
        cluster_partitions = get_cluster_partitions()
        for partition in cluster_partitions:
            partition_stats[partition] = {
                "pending": 0,
                "running": 0,
                "completed": 0
            }
        logger.debug(f"Initialized partition stats for partitions: {cluster_partitions}")
    else:
        logger.debug(f"Initialized partition stats from node data: {list(partition_stats.keys())}")

    # Process jobs (if any)
    for job in jobs:
        partition = job.get("partition")
        if not partition:
            logger.warning(f"Job {job.get('job_id', 'unknown')} has no partition specified, skipping")
            continue

        state = job.get("job_state", "UNKNOWN")
        job_id = job.get("job_id", "unknown")

        # Debug: Log each job's state
        logger.debug(f"Job {job_id} (partition {partition}): state={state} (type: {type(state)})")

        # Ensure partition exists in stats
        if partition not in partition_stats:
            partition_stats[partition] = {
                "pending": 0,
                "running": 0,
                "completed": 0
            }

        if _job_state_matches(state, "PENDING") or _job_state_matches(state, "CONFIGURING"):
            partition_stats[partition]["pending"] += 1
            logger.debug(f"Job {job_id} counted as PENDING")
        elif _job_state_matches(state, "RUNNING"):
            partition_stats[partition]["running"] += 1
            logger.debug(f"Job {job_id} counted as RUNNING")
        elif _job_state_matches(state, "COMPLETED") or _job_state_matches(state, "COMPLETING"):
            partition_stats[partition]["completed"] += 1
            logger.debug(f"Job {job_id} counted as COMPLETED")
        else:
            logger.debug(f"Job {job_id} with state '{state}' not counted in any category")

    # Debug: Log final statistics
    for partition, stats in partition_stats.items():
        logger.debug(f"Partition {partition} final stats: {stats}")

    # Process nodes
    node_stats = {}
    for node in node_data.get("nodes", []):
        partition = node.get("partition")
        if not partition:
            logger.warning(f"Node has no partition specified, skipping: {node}")
            continue

        state = node.get("state", "UNKNOWN")

        if partition not in node_stats:
            node_stats[partition] = {
                "total": 0,
                "available": 0
            }

        node_stats[partition]["total"] += 1
        if "idle" in state.lower():
            node_stats[partition]["available"] += 1

    # Combine stats
    results = []
    for partition in set(partition_stats.keys()) | set(node_stats.keys()):
        queue_stats = partition_stats.get(partition, {"pending": 0, "running": 0, "completed": 0})
        nodes = node_stats.get(partition, {"total": 0, "available": 0})

        results.append({
            "partition": partition,
            "queue_stats": queue_stats,
            "node_stats": nodes
        })

    return results


# Global variable to track current jobs
_CURRENT_JOBS = set()
_PROCESSED_COMPLETED_JOBS = set()
_LAST_CLEANUP_TIME = time.time()
_CLUSTER_PARTITIONS = None  # Cache for cluster partitions


def get_cluster_partitions():
    """Get the list of partitions from SLURM configuration"""
    global _CLUSTER_PARTITIONS

    if _CLUSTER_PARTITIONS is not None:
        return _CLUSTER_PARTITIONS

    try:
        # Get partitions from sinfo
        result = subprocess.run(
            [SINFO_CMD, "--format=%R", "--noheader"],
            capture_output=True,
            text=True,
            check=True,
            timeout=30
        )
        partitions = [line.strip() for line in result.stdout.strip().split('\n') if line.strip()]

        # Remove duplicates and filter out empty strings
        partitions = list(set([p for p in partitions if p]))

        if partitions:
            _CLUSTER_PARTITIONS = partitions
            logger.debug(f"Found cluster partitions: {partitions}")
            return partitions
        else:
            logger.warning("No partitions found in sinfo output, using 'batch' as default")
            _CLUSTER_PARTITIONS = ["batch"]
            return _CLUSTER_PARTITIONS

    except subprocess.CalledProcessError as e:
        logger.error(f"Failed to get partitions from sinfo: {e}")
    except subprocess.TimeoutExpired as e:
        logger.error(f"sinfo command timed out: {e}")
    except Exception as e:
        logger.error(f"Error getting cluster partitions: {e}")

    # Fallback to common partition names
    logger.warning("Could not get partitions from SLURM, using common defaults")
    _CLUSTER_PARTITIONS = ["batch", "compute", "debug"]
    return _CLUSTER_PARTITIONS


def cleanup_old_job_tracking():
    """Clean up old jobs from tracking that are older than 24 hours"""
    global _CURRENT_JOBS, _PROCESSED_COMPLETED_JOBS, _LAST_CLEANUP_TIME

    current_time = time.time()

    # Only run cleanup every hour
    if current_time - _LAST_CLEANUP_TIME < 3600:  # 1 hour
        return

    _LAST_CLEANUP_TIME = current_time

    # Clean up jobs older than 24 hours from current jobs tracking
    # This handles cases where jobs might get stuck in tracking
    if _CURRENT_JOBS:
        logger.debug(f"Cleaning up old job tracking. Current jobs: {_CURRENT_JOBS}")
        # Note: We can't easily determine job age from just the ID, so we'll rely on
        # the natural cleanup when jobs complete and are removed from tracking

    # Clean up processed completed jobs older than 24 hours
    # For now, we'll clean up all processed jobs every 24 hours to prevent memory buildup
    if _PROCESSED_COMPLETED_JOBS:
        logger.debug(f"Cleaning up processed completed jobs. Count: {len(_PROCESSED_COMPLETED_JOBS)}")
        _PROCESSED_COMPLETED_JOBS.clear()
        logger.debug("Cleared processed completed jobs cache")

    logger.debug("Job tracking cleanup completed")


def process_job_updates(queue_data, history_data):
    """Process job updates from queue and history data"""
    global _CURRENT_JOBS, _PROCESSED_COMPLETED_JOBS
    job_updates = []

    # Get current job IDs from queue data
    current_job_ids = set()
    if queue_data:
        for job in queue_data.get("jobs", []):
            job_id = job.get("job_id")
            if job_id:
                current_job_ids.add(job_id)

    # === STEP 1: Process jobs that have completed (were tracked but not in current queue) ===
    completed_job_ids = _CURRENT_JOBS - current_job_ids
    new_completed_job_ids = completed_job_ids - _PROCESSED_COMPLETED_JOBS

    if new_completed_job_ids:
        logger.info(f"Found newly completed jobs: {new_completed_job_ids}")

        # Try to find completion data in history first, then fallback to scontrol
        for job_id in new_completed_job_ids:
            job_update = None

            # Try to find in history data
            if history_data:
                for job in history_data.get("jobs", []):
                    if job.get("job_id") == job_id:
                        job_update = _create_job_update_from_data(job, "completed")
                        logger.info(f"Found completion data in history for job {job_id}")
                        break

            # Fallback to scontrol if not found in history
            if not job_update:
                logger.info(f"Job {job_id} not found in history, trying scontrol")
                job_state, exit_code, user_name = get_individual_job_status(job_id)
                if job_state:
                    job_update = {
                        "slurm_jobid": job_id,
                        "slurm_status": job_state,
                        "slurm_additional_states": [],
                        "slurm_start_time": None,
                        "slurm_end_time": None,
                        "user_name": user_name,
                        "partition": "unknown",
                        "nodes_allocated": None,
                        "ntasks_per_node": None,
                        "cpus_per_task": None,
                        "time_limit": None,
                        "name": f"Job {job_id}",
                        "exit_code": exit_code,
                        "update_type": "completed"
                    }
                    logger.info(f"Got completion data from scontrol for job {job_id}: {job_state}")
                else:
                    # Last resort: assume completed
                    logger.warning(f"Could not get status for job {job_id}, assuming COMPLETED")
                    job_update = {
                        "slurm_jobid": job_id,
                        "slurm_status": "COMPLETED",
                        "slurm_additional_states": [],
                        "slurm_start_time": None,
                        "slurm_end_time": None,
                        "user_name": "unknown",
                        "partition": "unknown",
                        "nodes_allocated": None,
                        "ntasks_per_node": None,
                        "cpus_per_task": None,
                        "time_limit": None,
                        "name": f"Job {job_id}",
                        "exit_code": None,
                        "update_type": "completed"
                    }

            if job_update:
                job_updates.append(job_update)
                _PROCESSED_COMPLETED_JOBS.add(job_id)

    # === STEP 2: Process currently active jobs ===
    if queue_data:
        for job in queue_data.get("jobs", []):
            job_id = job.get("job_id")
            if job_id:
                job_update = _create_job_update_from_data(job, "current")
                job_updates.append(job_update)

    # === STEP 3: Process untracked terminal jobs (immediately cancelled, etc.) ===
    if history_data:
        for job in history_data.get("jobs", []):
            job_id = job.get("job_id")
            # Check if this is a terminal job that we haven't processed yet
            # This includes immediately cancelled jobs that were never tracked
            if job_id and job_id not in _PROCESSED_COMPLETED_JOBS:
                # Extract job state from correct SLURM JSON field
                # Handle both formats: sacct uses state.current, squeue uses job_state
                job_state = None

                # Try sacct format first: state.current
                state_data = job.get("state", {})
                if isinstance(state_data, dict):
                    job_state = state_data.get("current", [])
                # Fallback to squeue format: job_state
                if not job_state:
                    job_state = job.get("job_state", [])

                # Check if it's a terminal state
                if job_state and any(state in ["COMPLETED", "FAILED", "CANCELLED", "TIMEOUT", "PREEMPTED", "NODE_FAIL"] for state in job_state):
                    # Skip if this job is currently active (avoid duplicates)
                    if job_id not in current_job_ids:
                        logger.info(f"Found unprocessed terminal job {job_id} in history: {job_state}")
                        job_update = _create_job_update_from_data(job, "completed")
                        job_updates.append(job_update)
                        _PROCESSED_COMPLETED_JOBS.add(job_id)

    # === STEP 4: Update global tracking ===
    # Remove completed jobs from tracking
    for job_id in new_completed_job_ids:
        _CURRENT_JOBS.discard(job_id)
        logger.debug(f"Removed completed job {job_id} from tracking")

    # Add new jobs to tracking
    _CURRENT_JOBS.update(current_job_ids)
    logger.debug(f"Updated job tracking: current jobs = {_CURRENT_JOBS}")

    return job_updates


def _create_job_update_from_data(job_data, update_type):
    """Create a standardized job update from SLURM job data"""
    job_id = job_data.get("job_id")

    # Extract job data using existing helper functions
    start_time, end_time = _extract_timestamps_from_job(job_data)
    job_state, exit_code = _determine_job_state_from_data(job_data, job_id)
    user_name = _extract_username_from_job(job_data, job_id)

    # Verify with scontrol for final status (if needed)
    if update_type == "completed":
        scontrol_state, scontrol_exit_code, scontrol_user = get_individual_job_status(job_id)
        if scontrol_state:
            job_state = scontrol_state
            exit_code = scontrol_exit_code
            if scontrol_user and not user_name:
                user_name = scontrol_user

    # Handle additional states for active jobs only
    additional_states = []
    if update_type == "current":
        # Handle both SLURM JSON formats for additional states:
        # - squeue --json uses: "job_state": ["RUNNING", "CONFIGURING"]  
        # - sacct --json uses: "state": {"current": ["CANCELLED"]}

        # Try sacct format first: state.current
        state_data = job_data.get("state", {})
        raw_job_state = None
        if isinstance(state_data, dict):
            raw_job_state = state_data.get("current", [])

        # Fallback to squeue format: job_state
        if not raw_job_state:
            raw_job_state = job_data.get("job_state", [])

        if isinstance(raw_job_state, list) and len(raw_job_state) > 1:
            if job_state not in ["COMPLETED", "FAILED", "CANCELLED", "TIMEOUT", "PREEMPTED"]:
                additional_states = raw_job_state[1:]
                logger.debug(f"Job {job_id} is active ({job_state}), including additional states: {additional_states}")

    return {
        "slurm_jobid": job_id,
        "slurm_status": job_state,
        "slurm_additional_states": additional_states,
        "slurm_start_time": start_time,
        "slurm_end_time": end_time,
        "user_name": user_name,
        "partition": job_data.get("partition"),
        "nodes_allocated": job_data.get("nodes_allocated"),
        "ntasks_per_node": job_data.get("ntasks_per_node"),
        "cpus_per_task": job_data.get("cpus_per_task"),
        "time_limit": job_data.get("time_limit"),
        "name": job_data.get("name"),
        "exit_code": exit_code,
        "update_type": update_type
    }


def slurm_monitoring_loop():
    """Background thread for SLURM monitoring"""
    logger.info("Starting SLURM monitoring background thread")

    while EXIT_CODE == 0:
        try:
            logger.debug("SLURM monitoring: Starting new monitoring cycle")

            # Clean up old job tracking
            cleanup_old_job_tracking()

            # Get SLURM status
            logger.debug("SLURM monitoring: Getting queue and node status")
            queue_data, node_data = get_slurm_queue_status()

            logger.debug("SLURM monitoring: Getting job history")
            try:
                history_data = get_recent_job_history()
                logger.debug(f"SLURM monitoring: History query successful, got data: {history_data is not None}")
            except Exception as history_error:
                logger.error(f"SLURM monitoring: Failed to get job history: {history_error}")
                logger.error(traceback.format_exc())
                history_data = None

            # Debug: Log what we got from SLURM
            logger.debug(f"SLURM monitoring: queue_data={queue_data is not None}, node_data={node_data is not None}, history_data={history_data is not None}")

            if queue_data:
                jobs = queue_data.get("jobs", [])
                logger.debug(f"SLURM monitoring: Got {len(jobs)} jobs from squeue --json")
                for job in jobs:
                    job_id = job.get("job_id")
                    job_state = job.get("job_state")
                    logger.debug(f"  Job {job_id}: {job_state}")
            else:
                logger.debug("SLURM monitoring: No current jobs in queue")

            # Debug: Log current tracking state
            logger.debug(f"SLURM monitoring: Currently tracking jobs: {_CURRENT_JOBS}")

            # Process queue data for statistics
            if queue_data and node_data:
                logger.debug("SLURM monitoring: Processing queue data for statistics")
                queue_stats = process_queue_data(queue_data, node_data)
                for stat in queue_stats:
                    send_message("SLURM_QUEUE_STATUS", {"data": stat})
            else:
                logger.debug("SLURM monitoring: No queue or node data for statistics")

            # Process job updates - this handles all job processing logic
            logger.debug("SLURM monitoring: Processing job updates")
            job_updates = process_job_updates(queue_data, history_data)
            logger.debug(f"SLURM monitoring: Generated {len(job_updates)} job updates")

            for update in job_updates:
                send_message("SLURM_JOB_UPDATE", {"data": update})

            # Debug: Log final tracking state
            logger.debug(f"SLURM monitoring: Final tracking state: {_CURRENT_JOBS}")
            logger.debug("SLURM monitoring: Completed monitoring cycle")

        except Exception as e:
            logger.error(f"Error in SLURM monitoring loop: {e}")
            logger.error(traceback.format_exc())

        # Sleep before next iteration
        logger.debug("SLURM monitoring: Sleeping for 30 seconds")
        time.sleep(30)

    logger.info("SLURM monitoring thread stopped")


def _spack_submit_build(app_id, partition, app_name, spec, extra_sbatch=None):
    build_dir = Path("/opt/cluster/installs") / str(app_id)
    build_dir.mkdir(parents=True, exist_ok=True)

    full_spec = f"{app_name}{spec}"

    outfile = build_dir / f"{app_name}.out"
    errfile = build_dir / f"{app_name}.err"

    extra_sbatch = (
        "\n".join([f"#SBATCH {e}" for e in extra_sbatch])
        if extra_sbatch
        else ""
    )

    script = build_dir / "install.sh"
    with script.open("w") as fileh:
        fileh.write(
            f"""#!/bin/bash
#SBATCH --partition={partition}
#SBATCH --nodes=1
#SBATCH --job-name={app_name}-install
#SBATCH --output={outfile.as_posix()}
#SBATCH --error={errfile.as_posix()}
{extra_sbatch}

# Suppress Python warnings that cause Spack to fail
export PYTHONWARNINGS="ignore"

cd {build_dir.as_posix()}
{spack_bin} install -v -y {full_spec}
"""
        )

    # Submit job
    try:
        proc = subprocess.run(
            ["sbatch", script.as_posix()],
            cwd=build_dir,
            check=True,
            encoding="utf-8",
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        if "Submitted batch job" in proc.stdout:
            jobid = int(proc.stdout.split()[-1])

            return (jobid, outfile, errfile)

        return (None, proc.stdout, proc.stderr)

    except subprocess.CalledProcessError as err:
        logger.error("sbatch exception", exc_info=err)
        return (None, err.stdout, err.stderr)


def _spack_confirm_install(app_name, log_file):
    """Return status of spack install

    Returns dict of 'status': ('r', 'e') (Ready, Error), and other data for
    database"""
    # Double-check that the install completed correctly
    results = {"status": "e"}
    try:
        with open(log_file, "r", encoding="utf-8") as fileh:
            last_line = fileh.readlines()[-1]
        if last_line.startswith("[+] /") and f"/{app_name}" in last_line:
            # Success
            results["status"] = "r"
            spack_hash = last_line.split("-")[-1].strip()
            results["spack_hash"] = spack_hash
            results["load_command"] = f"spack load /{spack_hash}"

        proc = subprocess.run(
            [spack_bin, "find", "--json", "--deps", f"/{spack_hash}"],
            check=True,
            stdout=subprocess.PIPE,
        )
        spack_json = json.loads(proc.stdout)

        compiler = spack_json[0]["compiler"]
        results["compiler"] = f"{compiler['name']} {compiler['version']}"

        arch = spack_json[0]["arch"]
        results["spack_arch"] = (
            f"{arch['platform']}-{arch['platform_os']}-{arch['target']['name']}"
            )

        # Look for MPI
        for dep in spack_json[1:]:
            if dep["name"] in [
                "intel-mpi",
                "intel-oneapi-mpi",
                "openmpi",
                "mpich",
                "cray-mpich",
                "fujitsu-mpi",
                "hpcx-mpi",
            ]:
                results["mpi"] = f"{dep['name']} {dep['version']}"

    except Exception as err:
        logger.error(
            "Failed to confirm Spack install of {app_name}", exc_info=err
        )

    return results


@cb_in_thread
def cb_spack_install(message):
    """Spack application installation handler"""

    ackid = message.get("ackid", None)
    appid = message.get("app_id", None)
    app_name = message["name"]
    logger.info(
        "Starting Spack Install for %s:%s - Message: %s",
        appid,
        app_name,
        message,
    )

    spack_stdout = f"/opt/cluster/installs/{appid}/{app_name}.out"
    spack_stderr = f"/opt/cluster/installs/{appid}/{app_name}.err"
    gcs_tgt_out = f"installs/{appid}/stdout"
    gcs_tgt_err = f"installs/{appid}/stderr"

    (jobid, outfile, errfile) = _spack_submit_build(
        appid,
        message["partition"],
        app_name,
        message["spec"],
        message["extra_sbatch"],
    )
    if not jobid:
        # There was an error - stdout, stderr in outfile, errfile
        logger.error(
            "Failed to run batch submission for %s:%s", appid, app_name
        )
        _upload_log_blobs(
            {
                gcs_tgt_out: outfile,
                gcs_tgt_err: errfile,
            }
        )
        send_message(
            "ACK",
            {"ackid": ackid, "app_id": appid, "jobid": jobid, "status": "e"},
        )
        return
    logger.info("Job Queued")
    send_message(
        "UPDATE",
        {"ackid": ackid, "app_id": appid, "jobid": jobid, "status": "q"},
    )

    state = "PENDING"
    while state in ["PENDING", "CONFIGURING"]:
        time.sleep(30)
        state = get_slurm_job_state(jobid)
    if state == "RUNNING":
        logger.info("Spack build job running for %s:%s", appid, app_name)
        send_message(
            "UPDATE",
            {"ackid": ackid, "app_id": appid, "jobid": jobid, "status": "i"},
        )
    while state in ["RUNNING"]:
        time.sleep(30)
        state = get_slurm_job_state(jobid)
        try:
            _upload_log_files(
                {gcs_tgt_out: spack_stdout, gcs_tgt_err: spack_stderr}
            )
        except Exception as err:
            logger.error(
                "Failed to upload log files for %s:%s",
                appid,
                app_name,
                exc_info=err,
            )
    logger.info(
        "Job for %s:%s completed with result %s", appid, app_name, state
    )
    status = "r" if state in ["COMPLETED", "COMPLETING"] else "e"
    final_update = {"ackid": ackid, "app_id": appid, "status": status}
    if status == "r":
        final_update.update(
            _spack_confirm_install(
                app_name, f"/opt/cluster/installs/{appid}/{app_name}.out"
            )
        )
    logger.info(
        "Uploading log files for %s:%s - (state: %s)",
        appid,
        app_name,
        final_update["status"],
    )
    try:
        _upload_log_files(
            {gcs_tgt_out: spack_stdout, gcs_tgt_err: spack_stderr}
        )
    except Exception as err:
        logger.error("Failed to upload log files", exc_info=err)
    send_message("ACK", final_update)


def _install_submit_job(app_id, partition, name, **message):
    build_dir = Path("/opt/cluster/installs") / str(app_id)
    build_dir.mkdir(parents=True, exist_ok=True)

    outfile = build_dir / f"{name}.out"
    errfile = build_dir / f"{name}.err"

    install_script = _make_run_script(
        build_dir, 0, 0, message["install_script"]
    )
    if not install_script:
        return (None, install_script, "Job not in recognized format")

    script = build_dir / "install_submit.sh"
    with script.open("w") as fileh:
        fileh.write(
            f"""#!/bin/bash
#SBATCH --partition={partition}
#SBATCH --nodes=1
#SBATCH --job-name={name}-install
#SBATCH --output={outfile.as_posix()}
#SBATCH --error={errfile.as_posix()}

cd {build_dir.as_posix()}

exec {install_script}
"""
        )

    # Submit job
    try:
        proc = subprocess.run(
            ["sbatch", script.as_posix()],
            cwd=build_dir,
            check=True,
            encoding="utf-8",
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        if "Submitted batch job" in proc.stdout:
            jobid = int(proc.stdout.split()[-1])
            return (jobid, outfile, errfile)

        return (None, proc.stdout, proc.stderr)

    except subprocess.CalledProcessError as err:
        logger.error("sbatch exception", exc_info=err)
        return (None, err.stdout, err.stderr)


@cb_in_thread
def cb_install_app(message):
    """Custom application installation handler"""

    appid = message["app_id"]
    app_name = message["name"]
    response = {"ackid": message["ackid"], "app_id": appid, "status": "e"}
    logger.info(
        "Beginning install of custom application %s:%s", appid, app_name
    )

    gcs_tgt_out = f"installs/{appid}/stdout"
    gcs_tgt_err = f"installs/{appid}/stderr"

    (jobid, outfile, errfile) = _install_submit_job(**message)
    if not jobid:
        # There was an error - stdout, stderr in outfile, errfile
        logger.error("Failed to run batch submission")
        _upload_log_blobs(
            {
                gcs_tgt_out: outfile,
                gcs_tgt_err: errfile,
            }
        )
        response["status"] = "e"
        send_message("ACK", response)
        return
    logger.info("Install job queued for %s:%s", appid, app_name)
    response["status"] = "q"
    send_message("UPDATE", response)

    state = "PENDING"
    while state in ["PENDING", "CONFIGURING"]:
        time.sleep(30)
        state = get_slurm_job_state(jobid)
    if state == "RUNNING":
        logger.info("Install job running for %s:%s", appid, app_name)
        response["status"] = "i"
        send_message("UPDATE", response)
    while state in ["RUNNING"]:
        time.sleep(30)
        state = get_slurm_job_state(jobid)
    logger.info(
        "Install job for %s:%s completed with result %s",
        appid,
        app_name,
        state,
    )
    status = "r" if state in ["COMPLETED", "COMPLETING"] else "e"
    response["status"] = status
    if status == "r":
        # Application installed.  Install Module file if appropriate
        if message.get("module_name", "") and message.get("module_script", ""):
            module_path = (
                Path("/opt/cluster/modulefiles") / message["module_name"]
            )
            module_path.parent.mkdir(parents=True, exist_ok=True)
            with module_path.open("w") as fileh:
                fileh.write(message["module_script"])

    logger.info(
        "Uploading install log files for %s:%s (state: %s)",
        appid,
        app_name,
        response["status"],
    )
    try:
        _upload_log_files(
            {
                gcs_tgt_out: f"/opt/cluster/installs/{appid}/{app_name}.out",
                gcs_tgt_err: f"/opt/cluster/installs/{appid}/{app_name}.err",
            }
        )
    except Exception as err:
        logger.error("Failed to upload log files", exc_info=err)
    send_message("ACK", response)


def _verify_oslogin_user(login_uid):
    # (username, uid, gid, homedir) = \
    #   _verify_oslogin_user(message['login_uid']):
    # Raises KeyError if login_uid not found in list
    global _OSLOGIN_CACHE
    if login_uid not in _OSLOGIN_CACHE:
        # pylint: disable=line-too-long
        # TODO - wrap in a loop with page Tokens

        req = requests.get(
                GCS_METADATA_BASEURL + "oslogin/users?pagesize=1024",
                headers=GCS_METADATA_HEADERS
                )
        resp = json.loads(req.text)
        _OSLOGIN_CACHE = {}
        for profile in resp["loginProfiles"]:
            uid = profile["name"]
            # TODO: Should also check login authorization
            for acct in profile["posixAccounts"]:
                if acct["primary"] or len(profile["posixAccounts"]) == 1:
                    _OSLOGIN_CACHE[uid] = (
                        acct["username"],
                        int(acct["uid"]),
                        int(acct["gid"]),
                        acct["homeDirectory"],
                    )
                    # Check to see if Homedir exists, and create if not
                    homedir_path = Path(acct["homeDirectory"])
                    if not homedir_path.is_dir():
                        logger.info(
                            "Creating homedir for user %s at %s",
                            acct["username"],
                            acct["homeDirectory"],
                        )
                        try:
                            subprocess.run(
                                ["mkhomedir_helper", acct["username"]],
                                check=True,
                            )
                        except Exception as err:
                            logger.error("Error creating homedir", exc_info=err)

    return _OSLOGIN_CACHE[login_uid]


def _verify_params(message, keys):
    for key in keys:
        if key not in message:
            return False
    return True


def _get_upload_command(target_dir, url):
    if url.startswith("gs://"):
        return f"gsutil cp -r '{target_dir.as_posix()}' '{url}'"
    if url.startswith("s3://"):
        return f"aws s3 cp --recursive '{target_dir.as_posix()}' '{url}'"

    raise NotImplementedError("Unsupported upload script_url scheme")


def _get_download_command(target_dir, url):
    if url.startswith("gs://"):
        return f"gsutil cp -r '{url}' '{target_dir.as_posix()}'"
    if url.startswith("s3://"):
        ret = f"""
output=$(aws s3 cp --recursive --dryrun '{url}' '{target_dir.as_posix()}')
if [ -z "$output" ]
then
    aws s3 cp '{url}' '{target_dir.as_posix()}'  # download a file
else
    aws s3 cp --recursive '{url}' '{target_dir.as_posix()}'  # download from folder recursively
fi"""
        return ret

    raise NotImplementedError("Unsupported upload script_url scheme")


def _make_run_script(job_dir, uid, gid, orig_run_script):
    text = orig_run_script.strip()
    script_url = urlparse(text)
    if text.startswith("#!"):  # Inline script
        text = text.replace("\r\n", "\n")  # Convert to UNIX line endings
        job_file = job_dir / "job.sh"
        with job_file.open("w", encoding="utf-8") as fileh:
            fileh.write(text)
            fileh.write("\n")
        job_file.chmod(0o755)
        os.chown(job_file, uid, gid)
        # Return just a call to this file to execute it
        return job_file.as_posix()

    if script_url.scheme in ["gs", "http", "https"]:
        recursive_fetch = script_url.path.endswith("/")
        fname = script_url.path.split("/")[-1] if not recursive_fetch else ""
        if script_url.scheme == "gs":
            fetch = (
                "gsutil "
                f"{'-m cp -r ' if recursive_fetch else ''}"
                f"'{text}' "
                f"'{job_dir.as_posix()}'"
            )
        elif script_url.scheme == "s3":
            fetch = (
                "aws s3 cp "
                f"{'--recursive ' if recursive_fetch else ''}"
                f"'{text}' "
                f"'{job_dir.as_posix()}'"
            )
        elif script_url.scheme in ["http", "https"]:
            if recursive_fetch:
                logger.error("Not Implemented recursive HTTP/HTTPS fetches")
                return None
            fetch = f"curl --silent -O '{text}'"

        if fname:
            extract = f"chmod 755 {fname}"
            execute = f"./{fname}"
            archive = False
            file_path = Path(fname)
            if file_path.suffixes in [
                [".tar", ".gz"],
                [".tar", ".xz"],
                [".tar", ".bz2"],
            ]:
                extract = f"tar xfa {file_path.name}"
                archive = True
            if file_path.suffixes in [[".zip"]]:
                extract = f"unzip {file_path.name}"
                archive = True
            if archive:
                execute = (
                    "# Find and execute most top-level 'run.sh' we can find\n"
                )
                execute += (
                    "$("
                    "find . -maxdepth 3 -name run.sh | "
                    "awk '{print length, $0}' | "
                    "sort -n  | "
                    "cut -d' ' -f2- | "
                    "head -n1"
                    ")"
                )
        return f"""
{fetch}
{extract}
{execute}
"""

    logger.error("Job Script not in a recognized format")
    return None


def _submit_job(
    uid,
    gid,
    job_dir,
    job_id,
    partition,
    num_nodes,
    run_script,
    *unused_args,
    **kwargs,
):
    """Returns (slurm_jobid, scriptFile, outfile, errfile)"""
    outfile = job_dir / "job.out"
    errfile = job_dir / "job.err"

    # TODO: Add things like ranksPerNode, threadsPerRank, wall time
    nranks = num_nodes
    extra_sbatch = ""

    is_container_job = kwargs.get("is_container_job", False)

    if is_container_job:
        logger.debug("Applying container-specific SBATCH configuration.")
        extra_sbatch += f"#SBATCH --cpus-per-task={kwargs['ranksPerNode']}\n"
    else:
        if "ranksPerNode" in kwargs:
            extra_sbatch += f"#SBATCH --ntasks-per-node={kwargs['ranksPerNode']}\n"
            nranks = num_nodes * kwargs["ranksPerNode"]
        if "threadsPerRank" in kwargs:
            extra_sbatch += f"#SBATCH --cpus-per-task={kwargs['threadsPerRank']}\n"

    if "wall_limit" in kwargs:
        extra_sbatch += f"#SBATCH --time={kwargs['wall_limit']}\n"
    if "gpus_per_node" in kwargs:
        extra_sbatch += f"#SBATCH --gpus={kwargs['gpus_per_node']}\n"

    # Container-specific settings
    if is_container_job:
        logger.debug("Applying container SBATCH lines because is_container_job=True")

        if "container_image_uri" in kwargs:
            token = subprocess.check_output(["gcloud", "auth", "print-access-token"]).strip().decode("utf-8")
            extra_sbatch += f"#SBATCH --container-image=docker://oauth2accesstoken:{token}@{kwargs['container_image_uri']}\n"

        if "container_mounts" in kwargs:
            extra_sbatch += f"#SBATCH --container-mounts={kwargs['container_mounts']}\n"

        if "container_envvars" in kwargs:
            extra_sbatch += f"#SBATCH --container-env={kwargs['container_envvars']}\n"

        if "container_workdir" in kwargs:
            extra_sbatch += f"#SBATCH --container-workdir={kwargs['container_workdir']}\n"

        # Booleans: set them explicitly if present, else skip
        if kwargs.get("container_writable", True):
            extra_sbatch += "#SBATCH --container-writable\n"
        else:
            extra_sbatch += "#SBATCH --container-readonly\n"

        if kwargs.get("container_use_entrypoint", False):
            extra_sbatch += "#SBATCH --container-entrypoint\n"
        else:
            extra_sbatch += "#SBATCH --no-container-entrypoint\n"

        if kwargs.get("container_remap_root", True):
            extra_sbatch += "#SBATCH --container-remap-root\n"
        else:
            extra_sbatch += "#SBATCH --no-container-remap-root\n"

        if kwargs.get("container_mount_home", True):
            extra_sbatch += "#SBATCH --container-mount-home\n"
        else:
            extra_sbatch += "#SBATCH --no-container-mount-home\n"
    else:
        logger.debug("Skipping container SBATCH lines because is_container_job=False or not present.")

    # Download input data, if specified
    download_command = ""
    upload_results = ""
    if kwargs.get("input_data", ""):
        download_command += _get_download_command(job_dir, kwargs["input_data"])
    if kwargs.get("result_data", ""):
        upload_results += _get_upload_command(job_dir, kwargs["result_data"])

    run_script = _make_run_script(job_dir, uid, gid, run_script)
    if not run_script:
        return (None, None, run_script, "Job not in recognized format")

    script = job_dir / "submit.sh"
    with script.open("w") as fileh:
        # Convert numbers into strings for sbatch
        u_name = pwd.getpwuid(uid).pw_name
        u_grname = grp.getgrgid(gid).gr_name
        spack_setup_line = ""
        if not is_container_job:
            spack_setup_line = f". {spack_path}/share/spack/setup-env.sh"

        fileh.write(
            f"""#!/bin/bash
#SBATCH --partition={partition}
#SBATCH --get-user-env
#SBATCH --uid={u_name}
#SBATCH --gid={u_grname}
#SBATCH --nodes={num_nodes}
#SBATCH --ntasks={1 if is_container_job else nranks}
#SBATCH --job-name=job_{job_id}
#SBATCH --output={outfile.as_posix()}
#SBATCH --error={errfile.as_posix()}
{extra_sbatch}

# Terminate on any script errors
set -e

cd {job_dir.as_posix()}

{spack_setup_line}
{kwargs.get('load_command', '')}

{download_command}

{run_script}
result=$?

{upload_results}

exit $result
"""
        )
    # Submit job
    try:
        proc = subprocess.run(
            ["sbatch", script.as_posix()],
            cwd=job_dir,
            check=True,
            encoding="utf-8",
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        if "Submitted batch job" in proc.stdout:
            jobid = int(proc.stdout.split()[-1])
            return (jobid, script, outfile, errfile)

        return (None, script, proc.stdout, proc.stderr)

    except subprocess.CalledProcessError as err:
        logger.error("sbatch exception", exc_info=err)
        return (None, script, err.stdout, err.stderr)


@cb_in_thread
def cb_run_job(message, **kwargs):
    """Handler for job submission and monitoring"""
    if not "ackid" in message:
        logger.error(
            "Refusing RUN_JOB message without ackid (message was %s)",
            message,
        )
        return
    ackid = message["ackid"]
    response = {"ackid": ackid}

    if not _verify_params(
        message, ["job_id", "login_uid", "run_script", "num_nodes", "partition"]
    ):
        logger.error("NOT STARTING JOB.  Missing required field(s)")
        response["status"] = "e"
        response["message"] = "Missing Key Info"
        send_message("ACK", response)
        return

    jobid = message["job_id"]
    response["job_id"] = jobid

    logger.info("Starting job %s:%s", jobid, message)

    if int(message["login_uid"]) == 0:
        (username, uid, gid, homedir) = ("root", 0, 0, "/home/root_jobs")
    else:
        try:
            (username, uid, gid, homedir) = _verify_oslogin_user(
                message["login_uid"]
            )
        except KeyError:
            logger.error(
                "User UID %s not OS-Login allowed", message["login_uid"]
            )
            response["status"] = "e"
            response["message"] = (
                f"User {username} (uid={uid}) is not allowed to submit jobs "
                "to this cluster"
            )
            send_message("ACK", response)
            return

    job_dir = Path(homedir) / "jobs" / str(jobid)
    job_dir.mkdir(parents=True, exist_ok=True)
    os.chown(job_dir, uid, gid)

    (slurm_jobid, script_path, outfile, errfile) = _submit_job(
        uid=uid, gid=gid, job_dir=job_dir, **message
    )
    if not slurm_jobid:
        # There was an error - stdout, stderr in outfile, errfile
        logger.error("Failed to run batch submission")
        _upload_log_blobs(
            {
                f"jobs/{jobid}/{script_path.name}": script_path.read_text(),
                f"jobs/{jobid}/stdout": outfile,
                f"jobs/{jobid}/stderr": errfile,
            }
        )
        response["status"] = "e"
        send_message("ACK", response)
        return
    logger.info("Job %s queued as slurm job %s", jobid, slurm_jobid)
    response["status"] = "q"
    response["slurm_job_id"] = slurm_jobid
    send_message("UPDATE", response)

    state = "PENDING"
    while state in ["PENDING", "CONFIGURING"]:
        time.sleep(30)
        state = get_slurm_job_state(slurm_jobid)

    if state == "RUNNING":
        logger.info("Job %s running as slurm job %s", jobid, slurm_jobid)
        response["status"] = "r"
        send_message("UPDATE", response)

    while state in ["RUNNING"]:
        time.sleep(30)
        state = get_slurm_job_state(slurm_jobid)

    logger.info(
        "Job %s (slurm %s) completed with result %s", jobid, slurm_jobid, state
    )
    status = "c" if state in ["COMPLETED", "COMPLETING"] else "e"
    response["status"] = "u"
    send_message("UPDATE", response)

    try:
        slurm_job_info = get_slurm_job_info(slurm_jobid)
        response["job_runtime"] = (
            slurm_job_info["end_time"]["number"] - slurm_job_info["start_time"]["number"]
        )
    except KeyError:
        logger.warning(
            "Job data from SLURM did not include start time and end time"
        )
    except Exception as E:
        logger.error("Unexpected error: %s", E)

    kpi = job_dir / "kpi.json"
    if kpi.is_file():
        with kpi.open("rb") as kpi_fh:
            kpi_info = json.load(kpi_fh)
            response.update(kpi_info)

    logger.info("Uploading log files for %s", jobid)
    try:
        _upload_log_files(
            {
                f"jobs/{jobid}/{script_path.name}": script_path.as_posix(),
                f"jobs/{jobid}/stdout": Path(job_dir / "job.out").as_posix(),
                f"jobs/{jobid}/stderr": Path(job_dir / "job.err").as_posix(),
            }
        )
    except Exception as err:
        logger.error("Failed to upload log files", exc_info=err)

    response["status"] = status
    send_message("ACK", response)

    if kwargs.get("cleanup_choice", "n") in [
        "a",
        "s" if status == "c" else "e",
    ]:
        # Need to empty the job dir before removing
        shutil.rmtree(job_dir)


@cb_in_thread
def cb_register_user_gcs(message, **kwargs):
    """Handle registration of user GCS credentials"""
    if not "ackid" in message:
        logger.error(
            "Refusing REGISTER_USER_GCS message without ackid (message was %s)",
            message,
        )
        return
    ackid = message["ackid"]
    response = {"ackid": ackid}

    logger.info("Starting REGISTER_USER_GCS: %s", message)

    try:
        (username, unused_uid, unused_gid, homedir) = _verify_oslogin_user(
            message["login_uid"]
        )
    except KeyError:
        logger.error(
            "User with uid=%s not OS-Login enabled", message["login_uid"]
        )
        response["status"] = "User does not have OS-Login permissions"
        response[
            "message"
        ] = "User is not allowed to submit jobs to this cluster"
        send_message("ACK", response)
        return

    try:
        response["status"] = "Configuring gcloud"
        send_message("UPDATE", response)
        subprocess.run(
            [
                "sudo",
                "-u",
                username,
                "gcloud",
                "config",
                "set",
                "pass_credentials_to_gsutil",
                "false",
            ],
            check=True,
        )

        # gsutil will fail if the backup file already exists
        boto_backup = Path(homedir) / ".boto.bak"
        if boto_backup.exists():
            boto_backup.unlink()

        with pexpect.spawn(
            "sudo",
            args=[
                "-u",
                username,
                "gsutil",
                "config",
                "-s",
                "https://www.googleapis.com/auth/devstorage.read_write",
            ],
        ) as child:
            child.expect(
                "Please navigate your browser to the following script_url:"
            )
            child.readline()  # Eat newline
            url = str(child.readline(), "utf-8").strip()
            response["status"] = "Waiting For User Auth"
            response["verify_url"] = url

            # Set up wait signal
            my_verify_key = None

            def my_callback(message):
                nonlocal my_verify_key
                my_verify_key = message.get("verify_key", None)

            _c2_ackMap[ackid] = my_callback

            send_message("UPDATE", response)
            response.pop("verify_url")

            # Wait for user to auth
            attempts = 0
            while not my_verify_key:
                time.sleep(2)
                attempts += 1
                if attempts > 150:  # 300 seconds
                    logger.error("Wait timed out - 5 minutes passed!")
                    response["status"] = "Wait timed out - 5 minutes passed!"
                    send_message("ACK", response)
                    child.terminate(force=True)
                    _c2_ackMap.pop(ackid)
                    return

            # Remove our callback, now that we have our verify key
            _c2_ackMap.pop(ackid)

            child.expect("Enter the authorization code:")
            child.sendline(my_verify_key)
            child.expect(pexpect.EOF)
            child.wait()
            child.close()
            response["exit_status"] = child.exitstatus
            response["status"] = (
                "Success" if child.exitstatus == 0 else "Failure"
            )
            send_message("ACK", response)

    except Exception as err:
        logger.error("Failed to configure User's GCS creds.", exc_info=err)
        send_message("ACK", response)


#  Other Callbacks


def cb_ping(message):
    """PING responder"""

    if "id" in message:
        pid = message["id"]
        logger.info("Received PING with id %s. Sending PONG", pid)
        send_message("PONG", {"id": pid})
    else:
        logger.info("Received anonymous PING: %s", message)


def cb_pong(message):
    """PONG receiver"""
    if "id" in message:
        pid = message["id"]
        logger.info("Received PONG with id %s", pid)
    else:
        logger.info("Received anonymous PONG: %s", message)


def cb_ack(message):
    """ACK handler"""
    ackid = message.get("ackid", None)
    logger.info("Received ACK to message %s", ackid)
    try:
        ack_callback = _c2_ackMap.pop(ackid)
        logger.info("Calling final callback for ACK id %s", ackid)
        ack_callback(message)
    except KeyError:
        pass


def cb_update(message):
    """UPDATE handler"""
    ackid = message.get("ackid", None)
    logger.info("Received UPDATE to message %s", ackid)
    try:
        update_callback = _c2_ackMap[ackid]
        logger.info("Calling callback for UPDATE to %s", ackid)
        update_callback(message)
    except KeyError:
        logger.warning("No registered Update Callback for id %s", ackid)


def cb_slurm_queue_status(message):
    """SLURM queue status handler - Forward to frontend"""
    # This is handled by the Django frontend, just forward the message
    send_message("SLURM_QUEUE_STATUS", message)


def cb_slurm_job_update(message):
    """SLURM individual job update handler - Forward to frontend"""
    # This is handled by the Django frontend, just forward the message
    send_message("SLURM_JOB_UPDATE", message)


callback_map = {
    "ACK": cb_ack,
    "PING": cb_ping,
    "PONG": cb_pong,
    "SYNC": cb_sync,
    "UPDATE": cb_update,
    "SPACK_INSTALL": cb_spack_install,
    "INSTALL_APPLICATION": cb_install_app,
    "RUN_JOB": cb_run_job,
    "REGISTER_USER_GCS": cb_register_user_gcs,
    "SLURM_QUEUE_STATUS": cb_slurm_queue_status,
    "SLURM_JOB_UPDATE": cb_slurm_job_update,
}


def callback_handler(message):
    """Handle callbacks from pubsub api

    This routine provides an entrypoint for the pubsub api and will call the
    appropriate real callback function based on the message `command` value.
    """

    # Avoid filling log with messages during normal operation
    # Do it this way using isEnabledFor avoids the (possible expensive) repr
    if logger.isEnabledFor(logging.DEBUG):
        logger.debug("Received message: %s", repr(message.data))
    cmd = message.attributes.get("command", None)
    if cmd in callback_map:
        callback_map[cmd](json.loads(message.data))
    else:
        if cmd:
            logger.warning(
                "Unknown command %s received in message, discarding", cmd
            )
        else:
            logger.warning("No Command attribute in the message, discarding")
    message.ack()


def _extract_username_from_job(job_data, job_id=None):
    """Extract username from job data, trying multiple field names"""
    user_name = job_data.get("user_name")
    if not user_name:
        user_name = job_data.get("user")
    if not user_name:
        user_name = job_data.get("user_id")
    if not user_name:
        user_name = job_data.get("account")

    # Debug logging for username extraction
    if logger.isEnabledFor(logging.DEBUG):
        logger.debug(f"Job {job_id} username fields: user_name={job_data.get('user_name')}, user={job_data.get('user')}, user_id={job_data.get('user_id')}, account={job_data.get('account')}, final={user_name}")

    return user_name


def _extract_timestamps_from_job(job_data):
    """Extract start and end timestamps from job data"""
    time_data = job_data.get("time", {})
    start_time = time_data.get("start") if time_data else None
    end_time = time_data.get("end") if time_data else None
    return start_time, end_time


def _determine_job_state_from_data(job_data, job_id=None):
    """Determine job state from job data, with fallback logic"""
    job_state = None

    # Handle both SLURM JSON formats:
    # - squeue --json uses: "job_state": ["RUNNING", "CONFIGURING"]  
    # - sacct --json uses: "state": {"current": ["CANCELLED"]}

    # Try sacct format first: state.current
    state_data = job_data.get("state", {})
    if isinstance(state_data, dict):
        raw_job_state = state_data.get("current")
        if isinstance(raw_job_state, list) and raw_job_state:
            job_state = raw_job_state[0]  # Take the first state

    # Fallback to squeue format: job_state
    if job_state is None:
        raw_job_state = job_data.get("job_state")
        if isinstance(raw_job_state, list) and raw_job_state:
            job_state = raw_job_state[0]  # Take the first state
        elif isinstance(raw_job_state, str):
            job_state = raw_job_state

    exit_code = job_data.get("exit_code")

    if job_state is None:
        time_data = job_data.get("time", {})
        end_time = time_data.get("end") if time_data else None
        if end_time and end_time > 0:
            job_state = "COMPLETED"
            logger.debug(f"Job {job_id} had no state but end time, assuming COMPLETED")
        else:
            job_state = "RUNNING"
            logger.debug(f"Job {job_id} had no state and no end time, assuming RUNNING")

    return job_state, exit_code


def get_individual_job_status(job_id):
    """Get job status and exit code using scontrol show job as fallback"""
    if not SCONTROL_CMD:
        return None, None, None

    try:
        result = subprocess.run(
            [SCONTROL_CMD, "show", "job", str(job_id)],
            capture_output=True,
            text=True,
            check=True,
            timeout=30
        )

        # Parse scontrol output
        output = result.stdout
        job_state = None
        exit_code = None
        user_name = None

        # Look for JobState=, ExitCode=, and UserName= in the output
        for line in output.split('\n'):
            line = line.strip()  # Remove leading/trailing whitespace
            if line.startswith('JobState='):
                # Extract job state, handling cases like "CANCELLED Reason=None"
                job_state_part = line.split('=')[1].strip()
                # Split on space to get just the state (e.g., "CANCELLED" from "CANCELLED Reason=None")
                job_state = job_state_part.split()[0] if job_state_part else None
            elif line.startswith('ExitCode='):
                exit_code_str = line.split('=')[1].strip()
                # ExitCode format is typically "0:0" (signal:exit_code) or "0:1" etc.
                if exit_code_str and exit_code_str != "0:0":
                    exit_code = exit_code_str
            elif line.startswith('UserName='):
                user_name = line.split('=')[1].strip()

        if job_state:
            logger.info(f"scontrol returned job state for {job_id}: {job_state}, exit_code: {exit_code}, user_name: {user_name}")
            # Special logging for cancelled jobs
            if job_state == 'CANCELLED':
                logger.info(f"*** DETECTED CANCELLED JOB {job_id} via scontrol ***")
        else:
            logger.debug(f"scontrol did not return job state for {job_id}, full output: {output}")

        return job_state, exit_code, user_name

    except subprocess.CalledProcessError as e:
        # Check if it's "Invalid job id" error (job completed and removed)
        if "Invalid job id" in e.stderr:
            logger.debug(f"Job {job_id} not found in scontrol (likely completed and removed)")
            return "COMPLETED", None, None  # Assume completed if job is not found
        else:
            logger.debug(f"Failed to get job status with scontrol for {job_id}: {e}")
            return None, None, None
    except Exception as e:
        logger.debug(f"Failed to get job status with scontrol for {job_id}: {e}")
        return None, None, None


def get_slurm_queue_status():
    """Get SLURM queue status using squeue command"""
    if not SQUEUE_CMD or not SINFO_CMD:
        logger.error("SLURM commands not found, cannot get queue status")
        return None, None

    try:
        # Get queue status
        result = subprocess.run(
            [SQUEUE_CMD, "--json"],
            capture_output=True,
            text=True,
            check=True,
            timeout=30
        )
        queue_data = json.loads(result.stdout)

        # Get node status
        node_result = subprocess.run(
            [SINFO_CMD, "--json"],
            capture_output=True,
            text=True,
            check=True,
            timeout=30
        )
        node_data = json.loads(node_result.stdout)

        return queue_data, node_data
    except subprocess.CalledProcessError as e:
        logger.error("Failed to get SLURM status: %s", e)
        return None, None
    except json.JSONDecodeError as e:
        logger.error("Failed to parse SLURM output: %s", e)
        return None, None
    except subprocess.TimeoutExpired as e:
        logger.error("SLURM command timed out: %s", e)
        return None, None


if __name__ == "__main__":

    streaming_pull_future = subscriber.subscribe(
        config["subscription_path"], callback=callback_handler
    )
    logger.info("Listening for messages on %s", config["subscription_path"])

    # Send cluster status update using the cluster_id from config
    send_message(
        "CLUSTER_STATUS",
        {
            "cluster_id": config["cluster_id"],
            "message": "Cluster C2 Daemon started",
            # Mark the cluster as now running
            "status": "r",
        },
    )

    # Start SLURM monitoring background thread
    slurm_monitor_future = thread_pool.submit(slurm_monitoring_loop)
    logger.info("Started SLURM monitoring background thread")

    # Wrap subscriber in a 'with' block to automatically call close() when done.
    with subscriber:
        try:
            # When `timeout` is not set, result() will block indefinitely,
            # unless an exception is encountered first.
            while EXIT_CODE == 0:
                try:
                    streaming_pull_future.result(timeout=10)
                except concurrent.futures.TimeoutError:
                    pass
            logger.info("Terminating with exit code %d", EXIT_CODE)

        except Exception as pull_err:
            logger.error(
                "Streaming Pull Received exception. Shutting down.",
                exc_info=pull_err,
            )
            EXIT_CODE = 1

        streaming_pull_future.cancel()  # Trigger the shutdown.
        # streaming_pull_future.result()  # Wait for finish

    thread_pool.shutdown(wait=True)

    send_message(
        "CLUSTER_STATUS",
        {
            "cluster_id": config["cluster_id"],
            "message": "Cluster C2 Daemon stopping",
        },
    )

    sys.exit(EXIT_CODE)
