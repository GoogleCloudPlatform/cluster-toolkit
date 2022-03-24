#!/usr/bin/env python3

from concurrent.futures import TimeoutError, ThreadPoolExecutor
from google.cloud import pubsub
from google.cloud import storage as gcs

import requests
import asyncio
import json, yaml, os, sys, shutil, pwd, grp
from functools import wraps
import time, subprocess, pexpect
from pathlib import Path
from urllib.parse import urlparse

import logging
import logging.handlers
logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)
# Send to syslog somehow
logger.addHandler(logging.handlers.SysLogHandler(address='/dev/log'))

# Set this to non-zero from a callback to cause us to exit
exit_code = 0

# Set the env var for testing
with open(os.environ.get('GHPCFE_CFG', '/usr/local/etc/ghpcfe_c2.yaml'), 'r') as cfg:
    config = yaml.safe_load(cfg)

source_id = f"cluster_{config['cluster_id']}"
cluster_bucket = config['cluster_bucket']
spack_path = config.get('spack_path', '/opt/cluster/spack')
spack_bin = f"{spack_path}/bin/spack"


pubClient = pubsub.PublisherClient()
subscriber = pubsub.SubscriberClient()
thread_pool = ThreadPoolExecutor()

_c2_ackMap = {}

def send_message(command, message, extra_attrs={}):
    # We always want our ID in the message
    message['cluster_id'] = config['cluster_id']
    pubClient.publish(config['topic_path'], bytes(json.dumps(message), 'utf-8'), command=command, source=source_id, **extra_attrs)


def cb_in_thread(func):
    @wraps(func)
    def wrapper(*args, **kwargs):
        logger.debug("Spawning Callback in threadpool")
        fut = thread_pool.submit(func, *args, **kwargs)
        try:
            logger.warning(f"Job had quick exception", exc_info=fut.exception(timeout=0.5))
        except TimeoutError:
            # Job is still going - good.
            pass
    return wrapper



def _download_gcs_directory(blob_path: str, tgtDir: Path) -> None:
    client = gcs.Client()
    gcs_bucket = client.bucket(cluster_bucket)
    for blob in client.list_blobs(gcs_bucket, prefix=blob_path):
        local_filename = tgtDir / blob.name[len(blob_path)+1:]
        logger.debug(f"Attempting to download {blob.name} from {cluster_bucket} to {local_filename.as_posix()}")
        local_filename.parent.mkdir(parents=True, exist_ok=True)
        blob.download_to_filename(local_filename.as_posix())


def _rerun_ansible():
    # Download ansible repo from GCS  (Can't just point at it)
    _download_gcs_directory(f"clusters/ansible_setup", Path("/tmp/ansible_setup"))
    
    logger.info("Downloaded Ansible Repo.  Beginning playbook")
    try:
        with open("/tmp/ansible_setup/hosts", "w") as fp:
            import socket
            fp.write(f"{socket.gethostname()}\n")
        with open("/tmp/ansible.log", "w") as ansible_log:
            proc = subprocess.run(["ansible-playbook", "./controller.yaml"], check=True,
                        cwd="/tmp/ansible_setup",
                        stdout=ansible_log, stderr=subprocess.STDOUT)
    except Exception as ex:
        logger.error("Ansible threw an error", exc_info=ex)
        raise
    finally:
        logger.info("Uploading ansible log file")
        _upload_log_files({"controller_logs/tmp/ansible.log": "/tmp/ansible.log"})



# Action functions

# @cb_in_thread
def cb_sync(message):
    logger.info(f"Starting sync:  Message: {message}")
    ackid = message.get('ackid', None)
    response = {'ackid': ackid}
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
    except Exception as ex:
        logger.error("Failed to upload log files", exc_info=ex)
        response['message'] = str(ex)

    # Download & run latest ansible config
    try:
        response['status'] = 'e'  # Suggest we're in an error'd state
        _rerun_ansible()
        response['status'] = 'r'  # Suggest we're in an error'd state
    except Exception as ex:
        logger.error("Failed to download & run ansible", exc_info=ex)
        response['message'] = str(ex)
    else:
        # Restart Daemon if ansible run was successful
        logger.info("Sending ACK and Attempting to restart c2 daemon")
        response['status'] = 'i'
        global exit_code
        exit_code = 123  # Magic code for systemctl to restart us
    finally:
        send_message('ACK', response)


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
    """ Returns the job state, or None if job isn't in the queue """
    # TODO - eventually, pyslurm might work with our version of Slurm,
    # and this can be changed to something more sane.  For now, call squeue
    try:
        proc = subprocess.run(["squeue", "--json"], check=True, stdout=subprocess.PIPE)
        output = json.loads(proc.stdout)
        for job in output['jobs']:
            if job['job_id'] == jobid:
                return job
        return None
    except Exception as ex:
        logger.error("Subprocess threw an error", exc_info=ex)
        return None


def _slurm_get_job_state(jobid):
    """ Returns the job state, or None if job isn't in the queue """
    # TODO - eventually, pyslurm might work with our version of Slurm,
    # and this can be changed to something more sane.  For now, call squeue
    state = _slurm_get_job_info(jobid)
    return state.get('job_state', None) if state else None


def _spack_submit_build(app_id, partition, app_name, spec):
    build_dir = Path('/opt/cluster/installs') / str(app_id)
    build_dir.mkdir(parents=True, exist_ok=True)

    full_spec = f"{app_name}{spec}"

    outfile = build_dir / f"{app_name}.out"
    errfile = build_dir / f"{app_name}.err"

    script = build_dir / 'install.sh'
    with script.open('w') as fp:
        fp.write(f"""#!/bin/bash
#SBATCH --partition={partition}
#SBATCH --nodes=1
#SBATCH --job-name={app_name}-install
#SBATCH --output={outfile.as_posix()}
#SBATCH --error={errfile.as_posix()}

cd {build_dir.as_posix()}
{spack_bin} install -v -y {full_spec}
""")

    # Submit job
    try:
        proc = subprocess.run(["sbatch", script.as_posix()],
                cwd=build_dir, check=True, encoding='utf-8',
                stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if "Submitted batch job" in proc.stdout:
            jobid = int(proc.stdout.split()[-1])
            return (jobid, outfile, errfile)
        else:
            return (None, proc.stdout, proc.stderr)
    except subprocess.CalledProcessError  as ex:
        logger.error("sbatch exception", exc_info=ex)
        return (None, ex.stdout, ex.stderr)

def _spack_confirm_install(app_name, log_file):
    """Returns dict of 'status': ('r', 'e') (Ready, Error), and other data for database"""
    # Double-check that the install completed correctly
    results = {'status': 'e'}
    try:
        with open(log_file, 'r') as fp:
            last_line = fp.readlines()[-1]
        if last_line.startswith('[+] /') and f"/{app_name}" in last_line:
            # Success
            results['status'] = 'r'
        spack_hash = last_line.split('-')[-1].strip()
        results['spack_hash'] = spack_hash
        results['load_command'] = f'spack load /{spack_hash}'

        proc = subprocess.run([spack_bin, "find", "--json", "--deps", f"/{spack_hash}"],
                check=True, stdout=subprocess.PIPE)
        spack_json = json.loads(proc.stdout)

        compiler = spack_json[0]['compiler']
        results['compiler'] = f"{compiler['name']} {compiler['version']}"

        arch = spack_json[0]['arch']
        results['spack_arch'] = f"{arch['platform']}-{arch['platform_os']}-{arch['target']['name']}"

        # Look for MPI
        for dep in spack_json[1:]:
            if dep['name'] in ["intel-mpi", "intel-oneapi-mpi", "openmpi", "mpich", "cray-mpich", "fujitsu-mpi", "hpcx-mpi"]:
                results['mpi'] = f"{dep['name']} {dep['version']}"

    except Exception as e:
        logger.error("Failed to confirm Spack install of {app_name}", exc_info=e)

    return results


@cb_in_thread
def cb_spack_install(message):
    logger.info(f"Starting Spack Install:  Message: {message}")
    ackid = message.get('ackid', None)
    appid = message.get('app_id', None)
    app_name = message['name']

    (jobid, outfile, errfile) = _spack_submit_build(appid, message['partition'], app_name, message['spec'])
    if not jobid:
        # There was an error - stdout, stderr in outfile, errfile
        logger.error("Failed to run batch submission")
        _upload_log_blobs({f"installs/{appid}/stdout": outfile,
                          f"installs/{appid}/stderr": errfile})
        send_message('ACK', {'ackid': ackid, 'app_id': appid, 'jobid': jobid, 'status': 'e'})
        return
    logger.info(f"Job Queued")
    send_message('UPDATE', {'ackid': ackid, 'app_id': appid, 'jobid': jobid, 'status': 'q'})

    state = "PENDING"
    while state in ["PENDING", "CONFIGURING"]:
        time.sleep(30)
        state = _slurm_get_job_state(jobid)
    if state == "RUNNING":
        logger.info(f"Job Running")
        send_message('UPDATE', {'ackid': ackid, 'app_id': appid, 'jobid': jobid, 'status': 'i'})
    while state in ["RUNNING"]:
        time.sleep(30)
        state = _slurm_get_job_state(jobid)
        try:
            _upload_log_files({
                f"installs/{appid}/stdout": f"/opt/cluster/installs/{appid}/{app_name}.out",
                f"installs/{appid}/stderr": f"/opt/cluster/installs/{appid}/{app_name}.err"})
        except Exception as ex:
            logger.error("Failed to upload log files", exc_info=ex)
    logger.info(f"Job Done with result {state}")
    status = 'r' if state in ["COMPLETED", "COMPLETING"] else 'e'
    final_update = {'ackid': ackid, 'app_id': appid, 'status': status}
    if status == 'r':
        final_update.update(_spack_confirm_install(app_name, f"/opt/cluster/installs/{appid}/{app_name}.out"))
    logger.info(f"Uploading Log files. [job state: {final_update['status']}]")
    try:
        _upload_log_files({
            f"installs/{appid}/stdout": f"/opt/cluster/installs/{appid}/{app_name}.out",
            f"installs/{appid}/stderr": f"/opt/cluster/installs/{appid}/{app_name}.err"})
    except Exception as ex:
        logger.error("Failed to upload log files", exc_info=ex)
    send_message('ACK', final_update)


def _install_submit_job(app_id, partition, name, **message):
    build_dir = Path('/opt/cluster/installs') / str(app_id)
    build_dir.mkdir(parents=True, exist_ok=True)

    outfile = build_dir / f"{name}.out"
    errfile = build_dir / f"{name}.err"

    install_script = _make_run_script(build_dir, 0, 0, message['install_script'])
    if not install_script:
        return (None, install_script, "Job not in recognized format")

    script = build_dir / 'install_submit.sh'
    with script.open('w') as fp:
        fp.write(f"""#!/bin/bash
#SBATCH --partition={partition}
#SBATCH --nodes=1
#SBATCH --job-name={name}-install
#SBATCH --output={outfile.as_posix()}
#SBATCH --error={errfile.as_posix()}

cd {build_dir.as_posix()}

exec {install_script}
""")

    # Submit job
    try:
        proc = subprocess.run(["sbatch", script.as_posix()],
                cwd=build_dir, check=True, encoding='utf-8',
                stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if "Submitted batch job" in proc.stdout:
            jobid = int(proc.stdout.split()[-1])
            return (jobid, outfile, errfile)
        else:
            return (None, proc.stdout, proc.stderr)
    except subprocess.CalledProcessError  as ex:
        logger.error("sbatch exception", exc_info=ex)
        return (None, ex.stdout, ex.stderr)


@cb_in_thread
def cb_install_app(message):
    logger.info(f"Starting install of application!")
    appid = message['app_id']
    app_name = message['name']
    response = {'ackid': message['ackid'], 'app_id': appid, 'status': 'e'}

    (jobid, outfile, errfile) = _install_submit_job(**message)
    if not jobid:
        # There was an error - stdout, stderr in outfile, errfile
        logger.error("Failed to run batch submission")
        _upload_log_blobs({f"installs/{appid}/stdout": outfile,
                          f"installs/{appid}/stderr": errfile})
        response['status'] = 'e'
        send_message('ACK', response)
        return
    logger.info(f"Job Queued")
    response['status'] = 'q'
    send_message('UPDATE', response)

    state = "PENDING"
    while state in ["PENDING", "CONFIGURING"]:
        time.sleep(30)
        state = _slurm_get_job_state(jobid)
    if state == "RUNNING":
        logger.info(f"Job Running")
        response['status'] = 'i'
        send_message('UPDATE', response)
    while state in ["RUNNING"]:
        time.sleep(30)
        state = _slurm_get_job_state(jobid)
    logger.info(f"Job Done with result {state}")
    status = 'r' if state in ["COMPLETED", "COMPLETING"] else 'e'
    response['status'] = status
    if status == 'r':
        # Application installed.  Install Module file if appropriate
        if message.get('module_name', '') and message.get('module_script', ''):
            fullModulePath = Path('/opt/cluster/modulefiles') / message['module_name']
            fullModulePath.parent.mkdir(parents=True, exist_ok=True)
            with fullModulePath.open('w') as fp:
                fp.write(message['module_script'])

    logger.info(f"Uploading Log files. [Spack state: {response['status']}]")
    try:
        _upload_log_files({
            f"installs/{appid}/stdout": f"/opt/cluster/installs/{appid}/{app_name}.out",
            f"installs/{appid}/stderr": f"/opt/cluster/installs/{appid}/{app_name}.err"})
    except Exception as ex:
        logger.error("Failed to upload log files", exc_info=ex)
    send_message('ACK', response)



_oslogin_cache = {}
def _verify_oslogin_user(login_uid):
#(username, uid, gid, homedir) = _verify_oslogin_user(message['login_uid']):
# Raises KeyError if login_uid not found in list
    global _oslogin_cache
    if login_uid not in _oslogin_cache:
        metadata_url = "http://metadata.google.internal/computeMetadata/v1/oslogin/users?pagesize=1024"
        metadata_headers = {'Metadata-Flavor': 'Google'}
        # TODO - wrap in a loop with page Tokens

        req = requests.get(metadata_url, headers=metadata_headers)
        resp = json.loads(req.text)
        _oslogin_cache = {}
        for profile in resp['loginProfiles']:
            uid = profile['name']
            # TODO: Should also check login authorization
            for acct in profile['posixAccounts']:
                if acct['primary'] or len(profile['posixAccounts'])==1:
                    _oslogin_cache[uid] = (acct['username'], int(acct['uid']), int(acct['gid']), acct['homeDirectory'])
                    # Check to see if Homedir exists, and create if not
                    homedirPath = Path(acct['homeDirectory'])
                    if not homedirPath.is_dir():
                        logger.info(f"Creating HomeDir for user {acct['username']} at {acct['homeDirectory']}")
                        try:
                            subprocess.run(["mkhomedir_helper", acct['username']])
                        except Exception as ex:
                            logger.error("Error creating homedir", exc_info=ex)
    
    return _oslogin_cache[login_uid]

def _verify_params(message, keys):
    for key in keys:
        if key not in message:
            return False
    return True


def _get_upload_command(tgtDir, url):
    if url.startswith('gs://'):
        return f"gsutil cp -r '{tgtDir.as_posix()}' '{url}'"
    elif url.startswith('s3://'):
        return f"aws s3 cp --recursive '{tgtDir.as_posix()}' '{url}'"
    else:
        raise NotImplementedError("Unsupported upload URL scheme")


def _get_download_command(tgtDir, url):
    if url.startswith('gs://'):
        return f"gsutil cp -r '{url}' '{tgtDir.as_posix()}'"
    elif url.startswith('s3://'):
        ret = f"""
output=$(aws s3 cp --recursive --dryrun '{url}' '{tgtDir.as_posix()}')
if [ -z "$output" ]
then
    aws s3 cp '{url}' '{tgtDir.as_posix()}'  # download a file
else
    aws s3 cp --recursive '{url}' '{tgtDir.as_posix()}'  # download from folder recursively
fi"""
        return ret
    else:
        raise NotImplementedError("Unsupported upload URL scheme")




def _make_run_script(job_dir, uid, gid, orig_run_script):
    text = orig_run_script.strip()
    URL = urlparse(text)
    if text.startswith("#!"):   # Inline script
        text = text.replace('\r\n', '\n')   # Convert to UNIX line endings
        jobFile = job_dir / 'job.sh'
        with jobFile.open('w') as fp:
            fp.write(text)
            fp.write('\n')
        jobFile.chmod(0o755)
        os.chown(jobFile, uid, gid)
        # Return just a call to this file to execute it
        return jobFile.as_posix()
    elif URL.scheme in ['gs', 'http', 'https']:
        recursive_fetch = URL.path.endswith('/')
        fname = URL.path.split('/')[-1] if not recursive_fetch else ''
        if URL.scheme == 'gs':
            fetch = f"gsutil{' -m cp -r' if recursive_fetch else ''} '{text}' '{job_dir.as_posix()}'"
        elif URL.scheme == 's3':
            fetch = f"aws s3 cp {'--recursive ' if recursive_fetch else ''} '{text}' '{job_dir.as_posix()}'"
        elif URL.scheme in ['http', 'https']:
            if recursive_fetch:
                logger.error("Not Implemented recursive HTTP/HTTPS fetchs")
                return None
            fetch = f"curl --silent -O '{text}'"


        if fname:
            extract = f"chmod 755 {fname}"
            execute = f"./{fname}"
            archive = False
            theFile = Path(fname)
            if theFile.suffixes in [['.tar', '.gz'], ['.tar', '.xz'], ['.tar', '.bz2']]:
                extract = f"tar xfa {theFile.name}"
                archive = True
            if theFile.suffixes in [['.zip']]:
                extract = f"unzip {theFile.name}"
                archive = True
            if archive:
                execute = "# Find and execute most top-level 'run.sh' we can find\n"
                execute += "$(find . -maxdepth 3 -name run.sh | awk '{print length, $0}' | sort -n  | cut -d' ' -f2- | head -n1)"

        return f"""
{fetch}
{extract}
{execute}
"""
    else:
        logger.error("Job Script not in a regonized format")
        return None


def _submit_job(uid, gid, job_dir, job_id, partition, nNodes, run_script, *args, **kwargs):
    """ Returns (slurm_jobid, scriptFile, outfile, errfile) """
    outfile = job_dir / "job.out"
    errfile = job_dir / "job.err"

    # TODO: Add things like ranksPerNode, threadsPerRank, wall time
    nranks = nNodes
    extra_sbatch = ""
    if 'ranksPerNode' in kwargs:
        extra_sbatch += f"#SBATCH --ntasks-per-node={kwargs['ranksPerNode']}\n"
        nranks = nNodes * kwargs['ranksPerNode']
    if 'threadsPerRank' in kwargs:
        extra_sbatch += f"#SBATCH --cpus-per-task={kwargs['threadsPerRank']}\n"
    if 'wall_limit' in kwargs:
        extra_sbatch += f"#SBATCH --time={kwargs['wall_limit']}\n"

    # Download input data, if specified
    download_command =""
    upload_results =""
    if kwargs.get("input_data", ''):
        download_command += _get_download_command(job_dir, kwargs["input_data"])
    if kwargs.get("result_data", ''):
        upload_results += _get_upload_command(job_dir, kwargs["result_data"])

    run_script = _make_run_script(job_dir, uid, gid, run_script)
    if not run_script:
        return (None, None, run_script, "Job not in recognized format")

    script = job_dir / 'submit.sh'
    with script.open('w') as fp:
        # Convert numbers into strings for sbatch
        u_name = pwd.getpwuid(uid).pw_name
        u_grname = grp.getgrgid(gid).gr_name
        fp.write(f"""#!/bin/bash
#SBATCH --partition={partition}
#SBATCH --get-user-env
#SBATCH --uid={u_name}
#SBATCH --gid={u_grname}
#SBATCH --nodes={nNodes}
#SBATCH --ntasks={nranks}
#SBATCH --job-name=job_{job_id}
#SBATCH --output={outfile.as_posix()}
#SBATCH --error={errfile.as_posix()}
{extra_sbatch}

# Terminate on any script errors
set -e

cd {job_dir.as_posix()}

. {spack_path}/share/spack/setup-env.sh
{kwargs.get('load_command', '')}

{download_command}

{run_script}
result=$?

{upload_results}

exit $result
""")
    # Submit job
    try:
        proc = subprocess.run(["sbatch", script.as_posix()],
                cwd=job_dir, check=True, encoding='utf-8',
                stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if "Submitted batch job" in proc.stdout:
            jobid = int(proc.stdout.split()[-1])
            return (jobid, script, outfile, errfile)
        else:
            return (None, script, proc.stdout, proc.stderr)
    except subprocess.CalledProcessError  as ex:
        logger.error("sbatch exception", exc_info=ex)
        return (None, script, ex.stdout, ex.stderr)


@cb_in_thread
def cb_run_job(message):
    logger.info(f"Starting Job Run:  Message: {message}")
    if not 'ackid' in message:
        logger.error("NOT STARTING JOB.  Message does not include 'ackid'")
        return
    ackid = message['ackid']
    response = {'ackid': ackid}

    if not _verify_params(message, ['job_id', 'login_uid', 'run_script', 'nNodes', 'partition']):
        logger.error("NOT STARTING JOB.  Missing required field(s)")
        response['status'] = 'e'
        response['message'] = 'Missing Key Info'
        send_message('ACK', response)
        return

    jobid = message['job_id']
    response['job_id'] = jobid

    if int(message['login_uid']) == 0:
        (username, uid, gid, homedir) = ('root', 0, 0, '/home/root_jobs')
    else:
        try:
            (username, uid, gid, homedir) = _verify_oslogin_user(message['login_uid'])
        except KeyError:
            logger.error(f"User UID {message['login_uid']} not OS-Login allowed")
            response['status'] = 'e'
            response['message'] = 'User is not allowed to submit jobs to this cluster'
            send_message('ACK', response)
            return

    job_dir = Path(homedir) / 'jobs' / str(jobid)
    job_dir.mkdir(parents=True, exist_ok=True)
    os.chown(job_dir, uid, gid)

    (slurm_jobid, scriptPath, outfile, errfile) = _submit_job(uid=uid, gid=gid, job_dir=job_dir, **message)
    if not slurm_jobid:
        # There was an error - stdout, stderr in outfile, errfile
        logger.error("Failed to run batch submission")
        _upload_log_blobs({f"jobs/{jobid}/{scriptPath.name}": scriptPath.read_text(),
                           f"jobs/{jobid}/stdout": outfile,
                           f"jobs/{jobid}/stderr": errfile})
        response['status'] = 'e'
        send_message('ACK', response)
        return
    logger.info(f"Job Queued")
    response['status'] = 'q'
    response['slurm_job_id'] = slurm_jobid
    send_message('UPDATE', response)

    state = "PENDING"
    while state in ["PENDING", "CONFIGURING"]:
        time.sleep(30)
        state = _slurm_get_job_state(slurm_jobid)

    if state == "RUNNING":
        logger.info(f"Job Running")
        response['status'] = 'r'
        send_message('UPDATE', response)

    while state in ["RUNNING"]:
        time.sleep(30)
        state = _slurm_get_job_state(slurm_jobid)

    logger.info(f"Job {jobid} Done with result {state}")
    status = 'c' if state in ["COMPLETED", "COMPLETING"] else 'e'
    response['status'] = 'u'
    send_message('UPDATE', response)

    try:
        slurm_job_info = _slurm_get_job_info(slurm_jobid)
        response['job_runtime'] = slurm_job_info['end_time'] - slurm_job_info['start_time']
    except KeyError:
        logger.warning("Job data from SLURM did not include start time and end time")

    kpi = job_dir / 'kpi.json'
    if kpi.is_file():
        with kpi.open('rb') as kp:
            kpi_info = json.load(kp)
            response.update(kpi_info)

    logger.info(f"Uploading Log files.")
    try:
        _upload_log_files({
            f"jobs/{jobid}/{scriptPath.name}": scriptPath.as_posix(),
            f"jobs/{jobid}/stdout": Path(job_dir / "job.out").as_posix(),
            f"jobs/{jobid}/stderr": Path(job_dir / "job.err").as_posix()})
    except Exception as ex:
        logger.error("Failed to upload log files", exc_info=ex)

    response['status'] = status
    send_message('ACK', response)

    if kwargs.get('cleanup_choice', 'n') in ['a', 's' if status == 'c' else 'e']:
        # Need to empty the job dir before removing
        shutil.rmtree(job_dir)


@cb_in_thread
def cb_register_user_gcs(message):
    logger.info(f"Starting Register User's GCS creds:  {message}")
    if not 'ackid' in message:
        logger.error("NOT STARTING JOB.  Message does not include 'ackid'")
        return
    ackid = message['ackid']
    response = {'ackid': ackid}

    try:
        (username, uid, gid, homedir) = _verify_oslogin_user(message['login_uid'])
    except KeyError:
        logger.error(f"User UID {message['login_uid']} not OS-Login allowed")
        response['status'] = 'User does not have OS-Login permissions'
        response['message'] = 'User is not allowed to submit jobs to this cluster'
        send_message('ACK', response)
        return


    try:
        response['status'] = "Configuring gcloud"
        send_message('UPDATE', response)
        subprocess.run(["sudo", "-u", username, "gcloud", "config", "set", "pass_credentials_to_gsutil", "false"])

        # gsutil will crap out if the backup file already exists
        bakFile = Path(homedir) / '.boto.bak'
        if bakFile.exists():
            bakFile.unlink()

        with pexpect.spawn("sudo", args=["-u", username, "gsutil", "config", "-s", "https://www.googleapis.com/auth/devstorage.read_write"]) as child:
            child.expect('Please navigate your browser to the following URL:')
            child.readline() # Eat newline
            url = str(child.readline(), 'utf-8').strip()
            response['status'] = 'Waiting For User Auth'
            response['verify_url'] = url

            # Set up wait signal
            my_verify_key = None
            def my_callback(message):
                nonlocal my_verify_key
                my_verify_key = message.get('verify_key', None)
            _c2_ackMap[ackid] = my_callback

            send_message('UPDATE', response)
            response.pop('verify_url')

            # Wait for user to auth
            attempts = 0
            while not my_verify_key:
                time.sleep(2)
                attempts += 1
                if attempts > 150: # 300 seconds
                    logger.error("Wait timed out - 5 minutes passed!")
                    response['status'] = 'Wait timed out - 5 minutes passed!'
                    send_message('ACK', response)
                    child.terminate(force=True)
                    _c2_ackMap.pop(ackid)
                    return

            # Remove our callback, now that we have our verify key
            _c2_ackMap.pop(ackid)

            child.expect('Enter the authorization code:')
            child.sendline(my_verify_key)
            child.expect(pexpect.EOF)
            child.wait()
            child.close()
            response['exit_status'] = child.exitstatus
            response['status'] = 'Success' if child.exitstatus == 0 else 'Failure'
            send_message('ACK', response)

    except Exception as ex:
        logger.error("Failed to configure User's GCS creds.", exc_info=ex)
        send_message('ACK', response)
    pass

#  Other Callbacks

def cb_ping(message):
    logger.info(f"Received PING")
    if 'id' in message:
        logger.info(f"    PING id {id}. Sending ACK")
        pid = message['id']
        send_message('PONG', {'id': pid})


def cb_pong(message):
    logger.info(f"Received PONG!")
    if 'id' in message:
        logger.info(f"    PING id {id}.")


def cb_ack(message):
    ackid = message.get('ackid', None)
    logger.info(f"Received ACK to message {ackid}!")
    try:
        cb = _c2_ackMap.pop[ackid]
        logger.info(f"Calling final callback for ACK id {ackid}")
        cb(message)
    except KeyError:
        pass


def cb_update(message):
    ackid = message.get('ackid', None)
    logger.info(f"Received UPDATE to message {ackid}!")
    try:
        cb = _c2_ackMap[ackid]
        logger.info("Calling callback for UPDATE")
        cb(message)
    except KeyError:
        logger.warning("No registered Update Callback for this ID")


callback_map = {
    'ACK': cb_ack,
    'PING': cb_ping,
    'PONG': cb_pong,
    'SYNC': cb_sync,
    'UPDATE': cb_update,
    'SPACK_INSTALL': cb_spack_install,
    'INSTALL_APPLICATION': cb_install_app,
    'RUN_JOB': cb_run_job,
    'REGISTER_USER_GCS': cb_register_user_gcs,
}


def callback(message):
    logger.info(f"Received {message.data!r}.")
    cmd = message.attributes.get('command', None)
    if cmd in callback_map:
        callback_map[cmd](json.loads(message.data))
    else:
        logger.warning("No Command attribute in the message.  Discarding")
    message.ack()


if __name__ == "__main__":

    streaming_pull_future = subscriber.subscribe(config['subscription_path'], callback=callback)
    logger.info(f"Listening for messages on {config['subscription_path']}..\n")

    send_message('CLUSTER_STATUS', {
        'message': "Cluster C2 Daemon started",
        # Mark the cluster as now running
        'status': 'r',
    })

    # Wrap subscriber in a 'with' block to automatically call close() when done.
    with subscriber:
        try:
            # When `timeout` is not set, result() will block indefinitely,
            # unless an exception is encountered first.
            while exit_code == 0:
                try:
                    streaming_pull_future.result(timeout=10)
                except TimeoutError:
                    pass
            logger.info(f"Exit Code set to {exit_code}.  Terminating")

        except Exception as exc:
            logger.error('Streaming Pull Received exception. Shutting down.', exc_info=exc)
            exit_code = 1

        streaming_pull_future.cancel()  # Trigger the shutdown.
        #streaming_pull_future.result()  # Wait for finish

    thread_pool.shutdown(wait=True)

    send_message('CLUSTER_STATUS', {
        'message': "Cluster C2 Daemon stopping",
    })

    sys.exit(exit_code)
