#!/usr/bin/env python3
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


import argparse
import os
import shelve
import uuid
from collections import namedtuple
from datetime import datetime, timedelta, timezone
from pathlib import Path
from pprint import pprint

import util
from google.api_core import exceptions, retry
from google.cloud import bigquery as bq
from google.cloud.bigquery import SchemaField
from util import lookup, run

SACCT = "sacct"
script = Path(__file__).resolve()

DEFAULT_TIMESTAMP_FILE = script.parent / "bq_timestamp"
timestamp_file = Path(os.environ.get("TIMESTAMP_FILE", DEFAULT_TIMESTAMP_FILE))
# The maximum request to insert_rows is 10MB, each sacct row is about 1200 bytes or ~ 8000 rows.
# Set to 5000 for a little wiggle room.
BQ_ROW_BATCH_SIZE = 5000

# cluster_id_file = script.parent / 'cluster_uuid'
# try:
# cluster_id = cluster_id_file.read_text().rstrip()
# except FileNotFoundError:
# cluster_id = uuid.uuid4().hex
# cluster_id_file.write_text(cluster_id)

job_idx_cache_path = script.parent / "bq_job_idx_cache"

SLURM_TIME_FORMAT = r"%Y-%m-%dT%H:%M:%S"


def make_datetime(time_string):
    if time_string == "None":
        return None
    return datetime.strptime(time_string, SLURM_TIME_FORMAT).replace(
        tzinfo=timezone.utc
    )


def make_time_interval(seconds):
    sign = 1
    if seconds < 0:
        sign = -1
        seconds = abs(seconds)
    d, r = divmod(seconds, 60 * 60 * 24)
    h, r = divmod(r, 60 * 60)
    m, s = divmod(r, 60)
    d *= sign
    h *= sign
    return f"{d}D {h:02}:{m:02}:{s}"


converters = {
    "DATETIME": make_datetime,
    "INTERVAL": make_time_interval,
    "STRING": str,
    "INT64": lambda n: int(n or 0),
}


def schema_field(field_name, data_type, description, required=False):
    return SchemaField(
        field_name,
        data_type,
        description=description,
        mode="REQUIRED" if required else "NULLABLE",
    )


schema_fields = [
    schema_field("cluster_name", "STRING", "cluster name", required=True),
    schema_field("cluster_id", "STRING", "UUID for the cluster", required=True),
    schema_field("entry_uuid", "STRING", "entry UUID for the job row", required=True),
    schema_field(
        "job_db_uuid", "INT64", "job db index from the slurm database", required=True
    ),
    schema_field("job_id_raw", "INT64", "raw job id", required=True),
    schema_field("job_id", "STRING", "job id", required=True),
    schema_field("state", "STRING", "final job state", required=True),
    schema_field("job_name", "STRING", "job name"),
    schema_field("partition", "STRING", "job partition"),
    schema_field("submit_time", "DATETIME", "job submit time"),
    schema_field("start_time", "DATETIME", "job start time"),
    schema_field("end_time", "DATETIME", "job end time"),
    schema_field("elapsed_raw", "INT64", "STRING", "job run time in seconds"),
    # schema_field("elapsed_time", "INTERVAL", "STRING", "job run time interval"),
    schema_field("timelimit_raw", "STRING", "job timelimit in minutes"),
    schema_field("timelimit", "STRING", "job timelimit"),
    # schema_field("num_tasks", "INT64", "number of allocated tasks in job"),
    schema_field("nodelist", "STRING", "names of nodes allocated to job"),
    schema_field("user", "STRING", "user responsible for job"),
    schema_field("uid", "INT64", "uid of job user"),
    schema_field("group", "STRING", "group of job user"),
    schema_field("gid", "INT64", "gid of job user"),
    schema_field("wckey", "STRING", "job wckey"),
    schema_field("qos", "STRING", "job qos"),
    schema_field("comment", "STRING", "job comment"),
    schema_field("admin_comment", "STRING", "job admin comment"),
    # extra will be added in 23.02
    # schema_field("extra", "STRING", "job extra field"),
    schema_field("exitcode", "STRING", "job exit code"),
    schema_field("alloc_cpus", "INT64", "count of allocated CPUs"),
    schema_field("alloc_nodes", "INT64", "number of nodes allocated to job"),
    schema_field("alloc_tres", "STRING", "allocated trackable resources (TRES)"),
    # schema_field("system_cpu", "INTERVAL", "cpu time used by parent processes"),
    # schema_field("cpu_time", "INTERVAL", "CPU time used (elapsed * cpu count)"),
    schema_field("cpu_time_raw", "INT64", "CPU time used (elapsed * cpu count)"),
    # schema_field("ave_cpu", "INT64", "Average CPU time of all tasks in job"),
    # schema_field(
    #    "tres_usage_tot",
    #    "STRING",
    #    "Tres total usage by all tasks in job",
    # ),
]


slurm_field_map = {
    "job_db_uuid": "DBIndex",
    "job_id_raw": "JobIDRaw",
    "job_id": "JobID",
    "state": "State",
    "job_name": "JobName",
    "partition": "Partition",
    "submit_time": "Submit",
    "start_time": "Start",
    "end_time": "End",
    "elapsed_raw": "ElapsedRaw",
    "elapsed_time": "Elapsed",
    "timelimit_raw": "TimelimitRaw",
    "timelimit": "Timelimit",
    "num_tasks": "NTasks",
    "nodelist": "Nodelist",
    "user": "User",
    "uid": "Uid",
    "group": "Group",
    "gid": "Gid",
    "wckey": "Wckey",
    "qos": "Qos",
    "comment": "Comment",
    "admin_comment": "AdminComment",
    # "extra": "Extra",
    "exit_code": "ExitCode",
    "alloc_cpus": "AllocCPUs",
    "alloc_nodes": "AllocNodes",
    "alloc_tres": "AllocTres",
    "system_cpu": "SystemCPU",
    "cpu_time": "CPUTime",
    "cpu_time_raw": "CPUTimeRaw",
    "ave_cpu": "AveCPU",
    "tres_usage_tot": "TresUsageInTot",
}

# new field name is the key for job_schema. Used to lookup the datatype when
# creating the job rows
job_schema = {field.name: field for field in schema_fields}
# Order is important here, as that is how they are parsed from sacct output
Job = namedtuple("Job", job_schema.keys())

client = bq.Client(
    project=lookup().cfg.project,
    credentials=util.default_credentials(),
    client_options=util.create_client_options(util.ApiEndpoint.BQ),
)
dataset_id = f"{lookup().cfg.slurm_cluster_name}_job_data"
dataset = bq.DatasetReference(project=lookup().project, dataset_id=dataset_id)
table = bq.Table(
    bq.TableReference(dataset, f"jobs_{lookup().cfg.slurm_cluster_name}"), schema_fields
)


class JobInsertionFailed(Exception):
    pass


def make_job_row(job):
    job_row = {
        field_name: dict.get(converters, field.field_type)(job[field_name])
        for field_name, field in job_schema.items()
        if field_name in job
    }
    job_row["entry_uuid"] = uuid.uuid4().hex
    job_row["cluster_id"] = lookup().cfg.cluster_id
    job_row["cluster_name"] = lookup().cfg.slurm_cluster_name
    return job_row


def load_slurm_jobs(start, end):
    states = ",".join(
        (
            "BOOT_FAIL",
            "CANCELLED",
            "COMPLETED",
            "DEADLINE",
            "FAILED",
            "NODE_FAIL",
            "OUT_OF_MEMORY",
            "PREEMPTED",
            "REQUEUED",
            "REVOKED",
            "TIMEOUT",
        )
    )
    start_iso = start.isoformat(timespec="seconds")
    end_iso = end.isoformat(timespec="seconds")
    # slurm_fields and bq_fields will be in matching order
    slurm_fields = ",".join(slurm_field_map.values())
    bq_fields = slurm_field_map.keys()
    cmd = (
        f"{SACCT} --start {start_iso} --end {end_iso} -X -D --format={slurm_fields} "
        f"--state={states} --parsable2 --noheader --allusers --duplicates"
    )
    text = run(cmd).stdout.splitlines()
    # zip pairs bq_fields with the value from sacct
    jobs = [dict(zip(bq_fields, line.split("|"))) for line in text]

    # The job index cache allows us to avoid sending duplicate jobs. This avoids a race condition with updating the database.
    with shelve.open(str(job_idx_cache_path), flag="r") as job_idx_cache:
        job_rows = [
            make_job_row(job)
            for job in jobs
            if str(job["job_db_uuid"]) not in job_idx_cache
        ]
    return job_rows


def init_table():
    global dataset
    global table
    dataset = client.create_dataset(dataset, exists_ok=True)
    table = client.create_table(table, exists_ok=True)
    until_found = retry.Retry(predicate=retry.if_exception_type(exceptions.NotFound))
    table = client.get_table(table, retry=until_found)
    # cannot add required fields to an existing schema
    table.schema = schema_fields
    table = client.update_table(table, ["schema"])


def purge_job_idx_cache():
    purge_time = datetime.now() - timedelta(minutes=30)
    with shelve.open(str(job_idx_cache_path), writeback=True) as cache:
        to_delete = []
        for idx, stamp in cache.items():
            if stamp < purge_time:
                to_delete.append(idx)
        for idx in to_delete:
            del cache[idx]


def bq_submit(jobs):
    try:
        result = client.insert_rows(table, jobs)
    except exceptions.NotFound as e:
        print(f"failed to upload job data, table not yet found: {e}")
        raise e
    except Exception as e:
        print(f"failed to upload job data: {e}")
        raise e
    if result:
        pprint(jobs)
        pprint(result)
        raise JobInsertionFailed("failed to upload job data to big query")
    print(f"successfully loaded {len(jobs)} jobs")


def get_time_window():
    if not timestamp_file.is_file():
        timestamp_file.touch()
    try:
        timestamp = datetime.strptime(
            timestamp_file.read_text().rstrip(), SLURM_TIME_FORMAT
        )
        # time window will overlap the previous by 10 minutes. Duplicates will be filtered out by the job_idx_cache
        start = timestamp - timedelta(minutes=10)
    except ValueError:
        # timestamp 1 is 1 second after the epoch; timestamp 0 is special for sacct
        start = datetime.fromtimestamp(1)
    # end is now() truncated to the last second
    end = datetime.now().replace(microsecond=0)
    return start, end


def write_timestamp(time):
    timestamp_file.write_text(time.isoformat(timespec="seconds"))


def update_job_idx_cache(jobs, timestamp):
    with shelve.open(str(job_idx_cache_path), writeback=True) as job_idx_cache:
        for job in jobs:
            job_idx = str(job["job_db_uuid"])
            job_idx_cache[job_idx] = timestamp


def main():
    if not lookup().cfg.enable_bigquery_load:
        print("bigquery load is not currently enabled")
        exit(0)
    init_table()

    start, end = get_time_window()
    jobs = load_slurm_jobs(start, end)
    # on failure, an exception will cause the timestamp not to be rewritten. So
    # it will try again next time. If some writes succeed, we don't currently
    # have a way to not submit duplicates next time.
    if jobs:
        num_batches = (len(jobs) - 1) // BQ_ROW_BATCH_SIZE + 1
        print(
            f"loading {num_batches} batches of BigQuery data in batches of size : {BQ_ROW_BATCH_SIZE}"
        )
        for batch_indx, job_indx in enumerate(range(0, len(jobs), BQ_ROW_BATCH_SIZE)):
            print(f"loading BigQuery data batch {batch_indx} of {num_batches}")
            bq_submit(jobs[job_indx : job_indx + BQ_ROW_BATCH_SIZE])
    write_timestamp(end)
    update_job_idx_cache(jobs, end)


parser = argparse.ArgumentParser(description="submit slurm job data to big query")
parser.add_argument(
    "timestamp_file",
    nargs="?",
    action="store",
    type=Path,
    help="specify timestamp file for reading and writing the time window start. Precedence over TIMESTAMP_FILE env var.",
)

purge_job_idx_cache()
if __name__ == "__main__":
    args = parser.parse_args()
    if args.timestamp_file:
        timestamp_file = args.timestamp_file.resolve()
    main()
