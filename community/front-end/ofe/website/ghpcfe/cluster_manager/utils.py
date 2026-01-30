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


def run_terraform(target_dir, command, arguments=None, extra_env=None):

    arguments = arguments if arguments else []
    extra_env = extra_env if extra_env else {}

    cmdline = ["terraform", command, "-no-color"]
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

    with log_out_fn.open("wb") as log_out:
        with log_err_fn.open("wb") as log_err:
            subprocess.run(
                cmdline,
                cwd=target_dir,
                env=new_env,
                stdout=log_out,
                stderr=log_err,
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
