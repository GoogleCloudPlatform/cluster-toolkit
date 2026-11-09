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

"""Commonly used utility routines"""

import copy
import datetime
import json
import logging
import os
import posixpath
import re
import secrets
import shlex
import socket
import subprocess
from pathlib import Path
import shutil

import yaml

logger = logging.getLogger(__name__)

# TODO = Make some form of global config file
g_baseDir = Path(__file__).resolve().parent.parent.parent.parent
g_config = {"baseDir": g_baseDir, "server": {}, "loaded": False}


def load_config(config_file=g_baseDir / "configuration.yaml", access_key=None):
    def _pathify(var):
        if var in g_config:
            if not isinstance(g_config[var], Path):
                g_config[var] = Path(g_config[var])

    if not g_config["loaded"]:

        with config_file.open("r") as f:
            g_config.update(yaml.safe_load(f)["config"])

        # Convert certain entries to appropriate types
        _pathify("baseDir")

        if access_key:
            g_config["server"]["access_key"] = access_key
        elif "C398_API_AUTHENTICATION_TOKEN" in os.environ:
            g_config["server"]["access_key"] = os.environ[
                "C398_API_AUTHENTICATION_TOKEN"
            ]

        g_config["loaded"] = True

    if access_key and (
        ("access_key" not in g_config["server"])
        or (access_key != g_config["server"]["access_key"])
    ):
        cfg = copy.deepcopy(g_config)
        cfg["server"]["access_key"] = access_key
        return cfg

    return g_config


# Terraform state prefixes by resource kind, placed under this database's
# state namespace (see get_state_namespace). They mirror the on-disk working
# directory names (clusters/cluster_<id>, fs/fs_<id>, ...), so an operator can
# map a prefix to its working directory by inspection.
TERRAFORM_STATE_PREFIXES = {
    "cluster": "clusters/cluster_{id}",
    "filesystem": "fs/fs_{id}",
    "vpc": "vpcs/vpc_{id}",
    "workbench": "workbenches/workbench_{id}",
    "image": "images/image_{id}",
}


def new_state_namespace():
    """Return a fresh state namespace: UTC creation date plus 8 random hex digits."""
    return (
        f"{datetime.datetime.now(datetime.timezone.utc):%Y%m%d}-"
        f"{secrets.token_hex(4)}"
    )


def get_state_namespace():
    """Return this database's state namespace, creating it on first use.

    Primary keys are unique only within one database, but the state bucket
    outlives it; the namespace stops a fresh database reusing an earlier
    database's prefixes.
    """
    # Imported here so this module stays importable without Django.
    from ..models import StateNamespace  # pylint: disable=import-outside-toplevel

    return StateNamespace.current()


def terraform_state_prefix(kind, resource_id, namespace=None):
    """Return the state-bucket prefix for one provisioned resource.

    `kind` is a key of TERRAFORM_STATE_PREFIXES; `resource_id` is the model's
    primary key. `namespace` defaults to this database's state namespace.
    """
    try:
        template = TERRAFORM_STATE_PREFIXES[kind]
    except KeyError as err:
        raise ValueError(
            f"No Terraform state prefix defined for resource kind {kind!r}. "
            f"Known kinds: {sorted(TERRAFORM_STATE_PREFIXES)}"
        ) from err
    if namespace is None:
        namespace = get_state_namespace()
    return f"{namespace}/{template.format(id=resource_id)}"


def get_terraform_state_bucket():
    """Return the GCS bucket holding Terraform state for provisioned resources.

    Set from the `terraform-state-bucket` instance metadata key by
    infrastructure_files/gcs_bucket/webserver/startup.sh. Missing configuration
    is fatal, not a fallback to local state: local state is lost with the
    instance, and Terraform cannot destroy a resource whose state is lost.
    """
    bucket = load_config()["server"].get("terraform_state_bucket")
    if not bucket:
        raise RuntimeError(
            "No 'terraform_state_bucket' in the server configuration. "
            "Terraform state must be stored in GCS; refusing to provision with "
            "local state. Check that the deployment set the "
            "'terraform-state-bucket' instance metadata key and that "
            "configuration.yaml was regenerated."
        )
    return bucket


def write_backend_tf(target_dir, prefix):
    """Write a GCS backend configuration into a Terraform working directory.

    For the resources copied from infrastructure_files/ (networks,
    workbenches); Cluster Toolkit deployments get their backend from
    `terraform_backend_defaults` in the blueprint.

    A file, unlike `-backend-config` at init, is found by every later
    terraform invocation in the directory, including ones that re-enter a
    directory created earlier (see vpc.create_subnet). Idempotent: an existing
    file is overwritten with the same content.
    """
    target_dir = Path(target_dir)
    backend_file = target_dir / "backend.tf"
    backend_file.write_text(
        "terraform {\n"
        '  backend "gcs" {\n'
        f'    bucket = "{get_terraform_state_bucket()}"\n'
        f'    prefix = "{prefix}"\n'
        "  }\n"
        "}\n",
        encoding="utf-8",
    )
    logger.info("Wrote GCS backend for prefix %s to %s", prefix, backend_file)
    return backend_file


def _parse_tfvars(filename):
    res = {}
    with open(filename, "r", encoding="utf-8") as fp:
        lines = list(fp)

    lnum = 0
    multi_line_terminator = None

    current_key = None
    current_value = None
    while lnum < len(lines):
        line = lines[lnum]
        lnum += 1
        if multi_line_terminator:
            if line.startswith(multi_line_terminator):
                multi_line_terminator = None
                res[current_key] = current_value
            else:
                current_value += line
        else:
            line = line.strip()
            if line.startswith("#"):
                continue
            line = line.split("=", maxsplit=1)
            if len(line) != 2:
                # Not sure what to do when it's not x=y... skip?
                continue
            (current_key, current_value) = [x.strip() for x in line]

            if current_value.startswith("<<"):
                multi_line_terminator = current_value[2:]
                current_value = ""
                continue

            res[current_key] = current_value.strip(' " " ')

    return res


def load_cluster_info(args):
    load_config(access_key=args.access_key)
    args.cluster_dir = g_baseDir / "clusters" / f"cluster_{args.cluster_id}"
    if not args.cluster_dir.is_dir():
        raise FileExistsError(f"Cluster ID {args.cluster_id} does not exist")

    if "cloud" not in args:
        for c in ["google"]:
            d = args.cluster_dir / "terraform" / c
            if d.is_dir():
                args.cloud = c
                break
        else:
            raise FileExistsError(
                "Unable to determine cloud type of cluster dir "
                f"{args.cluster_dir.as_posix()}"
            )

    # NOTE: this function has no caller, and the layout it expects
    # (<cluster_dir>/terraform/<cloud>) is not the one ClusterInfo produces.
    # It reads state from the backend only because the local state file no
    # longer exists; the layout mismatch is deliberately left alone.
    state = get_terraform_state(args.cluster_dir / "terraform" / args.cloud)
    args.cluster_ip = state["outputs"]["ManagementPublicIP"]["value"]
    args.cluster_name = state["outputs"]["cluster_id"]["value"]
    args.tf_state = state

    args.cluster_vars = _parse_tfvars(
        args.cluster_dir / "terraform" / args.cloud / "terraform.tfvars"
    )


def rsync_dir(
    source_dir,
    target_dir,
    args,
    log_dir,
    log_name="rsync_log",
    source_is_remote=False,
    rsync_opts=None,
):
    """
    Requires 'args.cluster_dir' and 'args.cluster_ip'
    """

    rsync_opts = rsync_opts if rsync_opts else []

    ssh_key = args.cluster_dir / ".ssh" / "id_rsa"
    ssh_args = f"ssh -i {ssh_key.as_posix()}"

    remote_str = f"citc@{args.cluster_ip}:"

    src_dir = (
        f'{(remote_str if source_is_remote else "")}{source_dir.as_posix()}/'
    )
    tgt_dir = (
        f'{(remote_str if not source_is_remote else "")}{target_dir.as_posix()}'
    )

    rsync_cmd = ["rsync", "-az", "--copy-unsafe-links", "-e", ssh_args]
    rsync_cmd.extend(rsync_opts)
    rsync_cmd.extend([src_dir, tgt_dir])

    new_env = os.environ.copy()
    # Don't have terraform try to reuse any existing SSH agent
    # It has its own keys
    if "SSH_AUTH_SOCK" in new_env:
        del new_env["SSH_AUTH_SOCK"]

    try:
        subprocess.run(
            rsync_cmd,
            env=new_env,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=True,
        )
    except subprocess.CalledProcessError as cpe:
        if cpe.stdout:
            with open(log_dir / f"{log_name}.stdout", "wb") as log_out:
                log_out.write(cpe.stdout)
        if cpe.stderr:
            with open(log_dir / f"{log_name}.stderr", "wb") as log_err:
                log_err.write(cpe.stderr)
        raise


def _terraform_env(extra_env=None):
    """Build the environment for a terraform invocation."""
    new_env = os.environ.copy()
    # Don't have terraform try to reuse any existing SSH agent
    # It has its own keys
    if "SSH_AUTH_SOCK" in new_env:
        del new_env["SSH_AUTH_SOCK"]
    new_env.update(extra_env if extra_env else {})
    return new_env


def get_terraform_state(target_dir, extra_env=None):
    """Return the Terraform state document for a working directory.

    Uses `terraform state pull`, because with a GCS backend there is no local
    terraform.tfstate; the output is the same raw state document.

    This contacts the backend, so `extra_env` must carry
    GOOGLE_APPLICATION_CREDENTIALS for the resource's provisioning credential.

    Not run through run_terraform(): it inserts -no-color directly after the
    command word, which `terraform state pull` rejects.
    """
    result = subprocess.run(
        ["terraform", "state", "pull"],
        cwd=target_dir,
        env=_terraform_env(extra_env),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=True,
    )
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as err:
        raise RuntimeError(
            f"Could not parse Terraform state pulled from {target_dir}: {err}"
        ) from err


# Terraform commands that take the state lock. With the GCS backend a second
# operation on one resource finds the lock held, and Terraform's default
# -lock-timeout of 0 makes it fail at once instead of waiting for the first.
TERRAFORM_LOCKING_COMMANDS = ("plan", "apply", "destroy", "import", "refresh")
# A cluster create or destroy can hold the lock for 20-40 minutes, so a
# shorter wait would still fail behind one.
TERRAFORM_LOCK_TIMEOUT = "45m"


class TerraformStateLockError(subprocess.CalledProcessError):
    """A resource's Terraform state lock could not be taken.

    Raised before Terraform runs when the lock is provably stale, and after
    Terraform gives up waiting for a lock. It is a CalledProcessError so that
    callers handle it exactly as any other failed Terraform run; the message
    is also carried in `stderr`, which those callers log.
    """

    def __init__(self, cmd, message, returncode=1):
        super().__init__(returncode, cmd, stderr=message.encode("utf-8"))
        self.message = message

    def __str__(self):
        return self.message


def _gcs_backend_config(target_dir):
    """Return (bucket, prefix) of an initialised directory's GCS backend.

    `terraform init` records the backend in .terraform/terraform.tfstate,
    whether it came from a written backend.tf or from the main.tf the Cluster
    Toolkit generates. Returns None if there is no such file or the backend
    is not GCS.
    """
    marker = Path(target_dir) / ".terraform" / "terraform.tfstate"
    try:
        backend = json.loads(marker.read_text()).get("backend") or {}
    except FileNotFoundError:
        return None
    if backend.get("type") != "gcs":
        return None
    config = backend.get("config") or {}
    return config["bucket"], config.get("prefix") or ""


def _gcs_client(credentials_file):
    # Imported here so this module stays importable without the Cloud
    # client libraries, which the Django-free tests rely on.
    from google.cloud import storage  # pylint: disable=import-outside-toplevel

    return storage.Client.from_service_account_json(credentials_file)


def _read_gcs_state_lock(bucket, prefix, credentials_file):
    """Return the lock info of the default workspace's state, or None.

    The GCS backend uses the lock object's generation as the lock ID, not the
    ID written inside the object, and force-unlock expects the generation, so
    "ID" is replaced with it.
    """
    name = posixpath.join(prefix.strip("/"), "default.tflock")
    blob = _gcs_client(credentials_file).bucket(bucket).get_blob(name)
    if blob is None:
        return None
    info = json.loads(blob.download_as_bytes())
    info["ID"] = str(blob.generation)
    return info


def _terraform_running_in(target_dir, proc_root="/proc"):
    """True if any terraform process on this machine runs in target_dir."""
    target = Path(target_dir).resolve()
    for proc in Path(proc_root).iterdir():
        if not proc.name.isdigit():
            continue
        try:
            if (proc / "comm").read_text().strip() != "terraform":
                continue
            if (proc / "cwd").resolve(strict=True) == target:
                return True
        except PermissionError:
            # Another user's terraform: its directory can't be read, so it
            # may hold this lock. Count it as running; the operation then
            # waits for the lock instead of calling it stale.
            return True
        except OSError:
            # Exited since the listing
            continue
    return False


def _short_hostname(name):
    return name.split(".")[0]


def _lock_age(created):
    try:
        then = datetime.datetime.fromisoformat(created)
    except (TypeError, ValueError):
        return "unknown"
    if then.tzinfo is None:
        then = then.replace(tzinfo=datetime.timezone.utc)
    age = datetime.datetime.now(datetime.timezone.utc) - then
    return str(datetime.timedelta(seconds=max(0, int(age.total_seconds()))))


def _state_lock_message(target_dir, info, credentials_file, reason):
    lock_id = info.get("ID") or "<lock ID>"
    credentials_file = credentials_file or "<resource credential file>"
    unlock = (
        f"cd {shlex.quote(str(target_dir))} && "
        f"GOOGLE_APPLICATION_CREDENTIALS={shlex.quote(str(credentials_file))} "
        f"terraform force-unlock -force {lock_id}"
    )
    return "\n".join([
        f"Terraform state for {target_dir} is locked. {reason}",
        f"  Lock ID:   {lock_id}",
        f"  Operation: {info.get('Operation', 'unknown')}",
        f"  Holder:    {info.get('Who', 'unknown')}",
        f"  Age:       {_lock_age(info.get('Created'))}"
        f" (created {info.get('Created', 'unknown')})",
        "Only if no Terraform operation is running for this resource, clear"
        " the lock with:",
        f"  {unlock}",
        "Never force-unlock while an operation is running. See \"A resource"
        " reports a stale state lock\" in docs/admin_guide.md.",
    ])


def _check_stale_state_lock(target_dir, cmdline, credentials_file):
    """Raise TerraformStateLockError if the state lock is provably stale.

    A lock goes stale only when Terraform is killed hard (server reboot, OOM,
    SIGKILL) while holding it. It is provably stale when it was taken on this
    machine and no terraform process here runs in this directory. A lock
    taken elsewhere, a live process, or any failure of this check is left to
    Terraform, which waits up to TERRAFORM_LOCK_TIMEOUT for it.

    This never removes the lock: removing a lock that is in use corrupts the
    state it protects.
    """
    if not credentials_file:
        return
    try:
        backend = _gcs_backend_config(target_dir)
        if backend is None:
            return
        info = _read_gcs_state_lock(*backend, credentials_file)
        if info is None:
            return
        _, sep, host = str(info.get("Who", "")).rpartition("@")
        if not sep or (
            _short_hostname(host) != _short_hostname(socket.gethostname())
        ):
            return
        if _terraform_running_in(target_dir):
            return
        # The holder may have finished and released the lock between the
        # read and the process scan; only the same lock still present counts.
        again = _read_gcs_state_lock(*backend, credentials_file)
        if again is None or again.get("ID") != info.get("ID"):
            return
    except Exception as err:  # pylint: disable=broad-except
        # Diagnostic only: it must never block a legitimate operation.
        logger.warning(
            "Could not check the Terraform state lock for %s; running"
            " Terraform anyway: %s", target_dir, err,
        )
        return

    message = _state_lock_message(
        target_dir, info, credentials_file,
        "The lock was taken on this server and no Terraform process on this"
        " server holds it, so it is stale: left by a Terraform run that was"
        " killed.",
    )
    logger.error(message)
    raise TerraformStateLockError(cmdline, message)


_LOCK_INFO_LINE = re.compile(
    r"^\s*(ID|Path|Operation|Who|Version|Created):\s*(.*?)\s*$", re.MULTILINE
)


def _raise_if_lock_timeout(err, target_dir, log_err_fn, credentials_file):
    """Turn a run that timed out waiting for the state lock into a clear error."""
    try:
        stderr = Path(log_err_fn).read_text(errors="replace")
    except OSError:
        return
    if "Error acquiring the state lock" not in stderr:
        return
    info = {}
    for key, value in _LOCK_INFO_LINE.findall(stderr):
        info.setdefault(key, value)
    message = _state_lock_message(
        target_dir, info, credentials_file,
        f"Terraform waited {TERRAFORM_LOCK_TIMEOUT} for the lock and gave up.",
    )
    logger.error(message)
    raise TerraformStateLockError(err.cmd, message, err.returncode) from err


def run_terraform(target_dir, command, arguments=None, extra_env=None):

    arguments = arguments if arguments else []
    extra_env = extra_env if extra_env else {}

    cmdline = ["terraform", command, "-no-color"]
    # Never prompt: these run unattended, and a prompt (e.g. init asking to
    # migrate state after a backend change) would block forever. With
    # -input=false, Terraform fails with its message instead.
    if command in ("init", "plan", "apply", "destroy", "import", "refresh"):
        cmdline.append("-input=false")
    if command in TERRAFORM_LOCKING_COMMANDS:
        cmdline.append(f"-lock-timeout={TERRAFORM_LOCK_TIMEOUT}")
    cmdline.extend(arguments)
    if command in ["apply", "destroy"]:
        cmdline.append("-auto-approve")

    log_out_fn = Path(target_dir) / f"terraform_{command}_log.stdout"
    log_err_fn = Path(target_dir) / f"terraform_{command}_log.stderr"

    new_env = _terraform_env(extra_env)

    if command in TERRAFORM_LOCKING_COMMANDS:
        _check_stale_state_lock(
            target_dir, cmdline, new_env.get("GOOGLE_APPLICATION_CREDENTIALS")
        )

    # Some Cluster Toolkit modules (e.g. the Slurm controller's compute-node
    # cleanup) run `gcloud` directly from an apply/destroy-time local-exec
    # provisioner. The gcloud CLI does not read GOOGLE_APPLICATION_CREDENTIALS
    # -- that only feeds Terraform's own provider and client libraries -- so
    # without an explicit gcloud auth context those provisioners silently run
    # as whatever account (if any) is ambiently active and typically do
    # nothing. Give this invocation its own isolated gcloud config, activated
    # with the same credential, so any local-exec that shells out to gcloud
    # is authenticated as the identity the operation was actually run with.
    creds_file = new_env.get("GOOGLE_APPLICATION_CREDENTIALS")
    if creds_file and command in ("apply", "destroy"):
        gcloud_config_dir = Path(target_dir) / ".gcloud_config"
        gcloud_config_dir.mkdir(exist_ok=True)
        new_env["CLOUDSDK_CONFIG"] = str(gcloud_config_dir)
        try:
            project_id = json.loads(Path(creds_file).read_text()).get("project_id")
        except (OSError, ValueError):
            project_id = None
        activate_cmd = [
            "gcloud", "--quiet", "auth", "activate-service-account",
            f"--key-file={creds_file}",
        ]
        if project_id:
            activate_cmd.append(f"--project={project_id}")
        try:
            subprocess.run(
                activate_cmd, env=new_env, check=True,
                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
            )
        except (subprocess.CalledProcessError, OSError) as err:
            # Best-effort: a module without a gcloud-dependent provisioner is
            # unaffected, and we must not block the terraform operation itself.
            logger.warning(
                "Could not activate gcloud auth context for %s: %s",
                target_dir, err,
            )

    with log_out_fn.open("wb") as log_out:
        with log_err_fn.open("wb") as log_err:
            try:
                subprocess.run(
                    cmdline,
                    cwd=target_dir,
                    env=new_env,
                    stdout=log_out,
                    stderr=log_err,
                    check=True,
                )
            except subprocess.CalledProcessError as err:
                if command in TERRAFORM_LOCKING_COMMANDS:
                    log_err.flush()
                    _raise_if_lock_timeout(
                        err, target_dir, log_err_fn,
                        new_env.get("GOOGLE_APPLICATION_CREDENTIALS"),
                    )
                raise

    return (log_out_fn, log_err_fn)

def run_packer(target_dir, command, arguments=None, extra_env=None):
    """
    Run the Packer command with the specified arguments in the given target directory.

    This function facilitates the execution of Packer commands from within a Python script.
    It uses the `subprocess.run` method to invoke Packer and logs the output and any errors
    generated during the execution.

    Args:
        target_dir (str): The target directory where Packer should be executed.
        command (str): The Packer command to be executed (e.g., "build", "validate", etc.).
        arguments (list, optional): Additional command-line arguments to pass to Packer.
                                    Defaults to an empty list if not provided.
        extra_env (dict, optional): Extra environment variables to set before running Packer.
                                    Defaults to an empty dictionary if not provided.

    Returns:
        tuple: A tuple containing two `Path` objects representing the paths to the log files
               where the standard output and standard error of the Packer command are logged.

    Raises:
        RuntimeError: If there is an error while executing the Packer command or if an
                      exception is raised during the execution.

    Note:
    - The `target_dir` should be a valid directory path where Packer-related files and
      configuration are located.
    - The `command` should be a valid Packer command (e.g., "build", "validate").
    - The `arguments` parameter allows users to provide additional command-line arguments
      for Packer as a list of strings.
    - The `extra_env` parameter allows users to set additional environment variables for
      Packer execution as a dictionary.
    - The standard output and standard error of the Packer command are logged to files in
      the target directory with filenames in the format `packer_{command}_log.stdout`
      and `packer_{command}_log.stderr`, respectively.

    Example Usage:
        >>> target_dir = "/path/to/packer/directory"
        >>> command = "build"
        >>> arguments = ["-var", "variable=value", "template.json"]
        >>> extra_env = {"PACKER_LOG": "1"}
        >>> run_packer(target_dir, command, arguments, extra_env)

    """
    arguments = arguments if arguments else []
    extra_env = extra_env if extra_env else {}

    # There is another binary called packer on the OS
    # To make sure we using correct packer specify full path
    cmdline = ["/usr/bin/packer", command]
    cmdline.extend(arguments)

    log_out_fn = Path(target_dir) / f"packer_{command}_log.stdout"
    log_err_fn = Path(target_dir) / f"packer_{command}_log.stderr"

    new_env = os.environ.copy()
    new_env.pop("SSH_AUTH_SOCK", None)
    new_env.update(extra_env)

    try:
        with log_out_fn.open("wb") as log_out, log_err_fn.open("wb") as log_err:
            subprocess.run(
                cmdline,
                cwd=target_dir,
                env=new_env,
                stdout=log_out,
                stderr=log_err,
                check=True,
            )
    except subprocess.CalledProcessError as e:
        # Handle the error from Packer command execution
        raise RuntimeError(f"Packer command failed: {e}")
    except Exception as E:
        # At this point catch any other exception as well.
        raise RuntimeError(f"Packer command failed: {E}")

    return (log_out_fn, log_err_fn)


def copy_file(source_file, destination_file):
    """
    Copy a file from the source path to the destination path.

    This function uses the `shutil.copy` method from the standard library to copy the file.
    It logs the success message if the file is copied successfully, and it logs any errors
    that may occur during the copy process.

    Args:
        source_file (str): The path to the source file that needs to be copied.
        destination_file (str): The path to the destination where the file should be copied.

    Raises:
        shutil.Error: If any error occurs during the file copy process.
        IOError: If there is an error while reading the source file or writing to the destination file.

    Note:
    - If the destination file already exists, it will be replaced by the source file.
    - If the source file does not exist, a `FileNotFoundError` will be raised by `shutil.copy`.

    Example:
        >>> source_file = "/path/to/source_file.txt"
        >>> destination_file = "/path/to/destination_file.txt"
        >>> copy_file(source_file, destination_file)

    """
    try:
        shutil.copy(source_file, destination_file)
        logger.info("File copied successfully.")
    except shutil.Error as e:
        logger.exception(f"Error occurred while copying the file: {e}")
    except IOError as e:
        logger.exception(f"Error occurred while reading or writing the file: {e}")
