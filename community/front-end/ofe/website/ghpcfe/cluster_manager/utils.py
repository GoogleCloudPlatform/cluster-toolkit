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
import glob
import json
import logging
import os
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


def config_file_path():
    """Path of the server configuration file `load_config` reads."""
    return g_baseDir / "configuration.yaml"


def invalidate_config_cache(config_file=None):
    """Re-read the server configuration into the cached document.

    Two properties of `load_config` shape this. It returns the module
    global itself rather than a copy, so anything holding the result --
    a running `ClusterInfo`, for one -- aliases this dict; and it merges
    the file in rather than replacing it, so a key that has since been
    deleted from the file would survive as a stale value.

    The cache is therefore refreshed in place: keys absent from the new
    document are dropped, then `load_config` repopulates the rest, which
    also reapplies its own `baseDir` conversion and access-key handling
    instead of duplicating them here. Emptying the dict outright would be
    wrong, because every alias would briefly see no configuration at all.
    A missing file leaves the cache alone and merely marks it unloaded,
    so a caller cannot destroy a working configuration by pointing this
    at a path that does not exist.
    """
    path = Path(config_file or config_file_path())
    try:
        with path.open("r", encoding="utf-8") as handle:
            fresh = (yaml.safe_load(handle) or {}).get("config") or {}
    except OSError:
        g_config["loaded"] = False
        return
    # baseDir is derived from the source location, never from the file.
    for stale in set(g_config) - set(fresh) - {"baseDir", "loaded"}:
        del g_config[stale]
    g_config["loaded"] = False
    load_config(config_file=path)


def update_config_file(mutate, config_file=None):
    """Apply `mutate` to the `config` mapping of the server config file.

    `mutate` receives the parsed `config` mapping and edits it in place.
    The file is rewritten through a temporary file in the same directory
    and then renamed, so a crash mid-write cannot leave the running
    server with a truncated configuration it can no longer parse. The
    memoised copy is dropped afterwards, since a caller that changed the
    file and kept serving the previous document would be worse than not
    having written at all.
    """
    config_file = Path(config_file or config_file_path())
    with config_file.open("r", encoding="utf-8") as handle:
        document = yaml.safe_load(handle) or {}
    section = document.setdefault("config", {})
    mutate(section)

    tmp_path = config_file.with_suffix(config_file.suffix + ".tmp")
    with tmp_path.open("w", encoding="utf-8") as handle:
        yaml.safe_dump(document, handle, default_flow_style=False,
                       sort_keys=False)
    os.replace(tmp_path, config_file)
    invalidate_config_cache()
    return document


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

    tf_state_file = (
        args.cluster_dir / "terraform" / args.cloud / "terraform.tfstate"
    )
    with tf_state_file.open("r") as statefp:
        state = json.load(statefp)
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


def terraform_may_have_applied(terraform_dir):
    """Whether Terraform may have created anything from a group directory.

    `ghpc create` writes each group's directory before any `terraform init`,
    so a run that failed in between leaves a directory nothing was applied
    from (or no directory at all), and a plain `terraform destroy` there only
    fails. OFE's blueprints set no Terraform backend, so state is local:
    `init` creates `.terraform` and `apply` writes `terraform.tfstate` (and
    its backup).
    """
    return (os.path.isdir(os.path.join(terraform_dir, ".terraform"))
            or bool(glob.glob(os.path.join(terraform_dir, "*.tfstate*"))))


def run_terraform(target_dir, command, arguments=None, extra_env=None):

    arguments = arguments if arguments else []
    extra_env = extra_env if extra_env else {}

    # never let terraform fall back to an interactive prompt for a
    # missing variable. This helper runs unattended inside a backend async
    # task with no one to answer stdin, so a missing required variable
    # (e.g. from an upstream failure that skipped writing tfvars) hangs
    # the subprocess forever holding the state lock, instead of failing
    # fast with a normal "no value for required variable" error.
    #
    # only the subcommands that actually evaluate variables accept
    # an `-input` flag. Read-only commands (`validate`, `output`, ...) reject
    # it outright ("flag provided but not defined: -input") and fail. Use an
    # allowlist of the variable-evaluating writers rather than a denylist, so
    # a future read-only subcommand is not silently broken again.
    cmdline = ["terraform", command, "-no-color"]
    if command in ("init", "plan", "apply", "destroy", "refresh", "import"):
        cmdline.append("-input=false")
    cmdline.extend(arguments)
    if command in ["apply", "destroy"]:
        cmdline.append("-auto-approve")

    log_out_fn = Path(target_dir) / f"terraform_{command}_log.stdout"
    log_err_fn = Path(target_dir) / f"terraform_{command}_log.stderr"

    new_env = os.environ.copy()
    # Don't have terraform try to reuse any existing SSH agent
    # It has its own keys
    if "SSH_AUTH_SOCK" in new_env:
        del new_env["SSH_AUTH_SOCK"]
    new_env.update(extra_env)

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
            subprocess.run(
                cmdline,
                cwd=target_dir,
                env=new_env,
                stdout=log_out,
                stderr=log_err,
                stdin=subprocess.DEVNULL,
                check=True,
            )

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
