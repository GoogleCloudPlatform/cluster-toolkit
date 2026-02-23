#!/usr/bin/env python3
# Copyright 2026 Google LLC
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


def _slurm_get_job_info(jobid):
    """Returns the job state, or None if job isn't in the queue"""
    # N.B - eventually, pyslurm might work with our version of Slurm,
    # and this can be changed to something more sane.  For now, call squeue
    try:
        proc = subprocess.run(
            ["squeue", "--json"], check=True, stdout=subprocess.PIPE
        )
        output = json.loads(proc.stdout)
        for job in output["jobs"]:
            if job["job_id"] == jobid:
                return job
        return None
    except Exception as err:
        logger.error("Subprocess threw an error", exc_info=err)
        return None


def _slurm_get_job_state(jobid):
    """Returns the job state, or None if the job isn't in the queue"""
    state = _slurm_get_job_info(jobid)  # Fetch job info using an external function
    job_state = state.get("job_state", None) if state else None  # Get the 'job_state' if available

    if job_state and isinstance(job_state, list) and job_state:
        logger.info("Slurm returned job %s with state %s", jobid, job_state[0])  # Log the first state if available
        return job_state[0]  # Return the first element of the state list
    else:
        logger.info("No valid job state available for job %s", jobid)  # Log when no valid state is found

    return None  # Return None if there is no job state or it's not a list

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
        state = _slurm_get_job_state(jobid)
    if state == "RUNNING":
        logger.info("Spack build job running for %s:%s", appid, app_name)
        send_message(
            "UPDATE",
            {"ackid": ackid, "app_id": appid, "jobid": jobid, "status": "i"},
        )
    while state in ["RUNNING"]:
        time.sleep(30)
        state = _slurm_get_job_state(jobid)
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
        state = _slurm_get_job_state(jobid)
    if state == "RUNNING":
        logger.info("Install job running for %s:%s", appid, app_name)
        response["status"] = "i"
        send_message("UPDATE", response)
    while state in ["RUNNING"]:
        time.sleep(30)
        state = _slurm_get_job_state(jobid)
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
        state = _slurm_get_job_state(slurm_jobid)

    if state == "RUNNING":
        logger.info("Job %s running as slurm job %s", jobid, slurm_jobid)
        response["status"] = "r"
        send_message("UPDATE", response)

    while state in ["RUNNING"]:
        time.sleep(30)
        state = _slurm_get_job_state(slurm_jobid)

    logger.info(
        "Job %s (slurm %s) completed with result %s", jobid, slurm_jobid, state
    )
    status = "c" if state in ["COMPLETED", "COMPLETING"] else "e"
    response["status"] = "u"
    send_message("UPDATE", response)

    try:
        slurm_job_info = _slurm_get_job_info(slurm_jobid)
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


if __name__ == "__main__":

    streaming_pull_future = subscriber.subscribe(
        config["subscription_path"], callback=callback_handler
    )
    logger.info("Listening for messages on %s", config["subscription_path"])

    send_message(
        "CLUSTER_STATUS",
        {
            "message": "Cluster C2 Daemon started",
            # Mark the cluster as now running
            "status": "r",
        },
    )

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
            "message": "Cluster C2 Daemon stopping",
        },
    )

    sys.exit(EXIT_CODE)
