#!/slurm/python/venv/bin/python3.13

# Copyright 2025 Google LLC
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

import subprocess
import shlex
import logging
import functools
import requests
from pathlib import Path
import shutil
from dataclasses import dataclass
import re
from google.cloud import storage


log = logging.getLogger()


def log_subprocess(subj: subprocess.CalledProcessError | subprocess.TimeoutExpired | subprocess.CompletedProcess) -> None:
    match subj:
        case subprocess.CompletedProcess(returncode=0):
            # Do not log successful runs, to not overwhelm logs (e.g. scontrol show jobs --json)
            # TODO: consider still doing it in DEBUG or trim output to few KBs. 
            return
        case subprocess.CompletedProcess(): # non-zero returncode
            log.error(f"Command '{subj.args}' returned exit status {subj.returncode}.")
        case subprocess.CalledProcessError() | subprocess.TimeoutExpired():
            log.error(str(subj))


def run(
    args:str,
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
    shell=False,
    timeout=None,
    check=True,
    universal_newlines=True,
    **kwargs,
):
    """Wrapper for subprocess.run() with convenient defaults"""
    if not shell:
        args = shlex.split(args)
    log.debug(f"run: {args}")
    try:
        result = subprocess.run(
            args,
            stdout=stdout,
            stderr=stderr,
            shell=shell,
            timeout=timeout,
            check=check,
            universal_newlines=universal_newlines,
            **kwargs,
        )
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired) as e:
        log_subprocess(e)
        raise
    log_subprocess(result)
    return result


def chown_slurm(path: Path, mode=None) -> None:
    if mode:
        path.chmod(mode)
    shutil.chown(path, user="slurm", group="slurm")


METADATA_ROOT_URL = "http://metadata.google.internal/computeMetadata/v1"

class MetadataNotFoundError(Exception):
    pass

def get_metadata(path:str, silent=False) -> str:
    """Get metadata relative to metadata/computeMetadata/v1"""
    headers = {"Metadata-Flavor": "Google"}
    url = f"{METADATA_ROOT_URL}/{path}"
    try:
        resp = requests.get(url, headers=headers)
        resp.raise_for_status()
        return resp.text
    except requests.exceptions.HTTPError:
        if not silent:
            log.warning(f"metadata not found ({url})")
        raise MetadataNotFoundError(f"failed to get_metadata from {url}")


@functools.lru_cache(maxsize=None)
def instance_metadata(path: str, silent:bool=False) -> str:
    return get_metadata(f"instance/{path}", silent=silent)


def instance_role():
    return instance_metadata("attributes/slurm_instance_role")


def instance_login_group():
    return instance_metadata("attributes/slurm_login_group")

def instance_nodeset():
    return instance_metadata("attributes/slurm_nodeset")

def cluster_name():
    return instance_metadata("attributes/slurm_cluster_name")

def controller_host():
    return f"{cluster_name()}-controller" # !!!!


def install_custom_scripts(check_hash:bool=False):
    """download custom scripts from gcs bucket"""
    role, tokens = instance_role(), []


    if role == "compute":
        tokens = [f"nodeset-{instance_nodeset()}", "prolog", "epilog", "task_prolog", "task_epilog"]
    elif role == "login":
        tokens = [f"login-{instance_login_group()}"]

    prefixes = [f"slurm-{tok}-script" for tok in tokens]

    
    source_collection = list(chain.from_iterable(blob_list(prefix=p) for p in prefixes))

    script_pattern = re.compile(r"^slurm-(?P<path>\S+)-script-(?P<name>\S+)")
    for source in source_collection:
        m = script_pattern.match(Path(source.name).name)

        if not m:
            log.warning(f"found blob that doesn't match expected pattern: {source.name}")
            continue
        path_parts = m["path"].split("-")
        path_parts[0] += ".d"
        stem, _, ext = m["name"].rpartition("_")
        filename = ".".join((stem, ext))

        path = Path(*path_parts, filename)
        fullpath = (dirs.custom_scripts / path).resolve()
        mkdirp(fullpath.parent)

        for par in path.parents:
            chown_slurm(dirs.custom_scripts / par)
        need_update = True

        if check_hash and fullpath.exists():
            # TODO: MD5 reported by gcloud may differ from the one calculated here (e.g. if blob got gzipped),
            # consider using gCRC32C
            need_update = hash_file(fullpath) != source.md5_hash

        if need_update:
            log.info(f"installing custom script: {path} from {source.name}")
            with fullpath.open("wb") as f:
                source.download_to_file(f)
            chown_slurm(fullpath, mode=0o755)


@dataclass(frozen=True)
class NSMount:
    server_ip: str
    local_mount: Path
    remote_mount: Path
    fs_type: str
    mount_options: str


@dataclass
class Config:
    network_storage: list[NSMount]
    startup_script_timeout: int = 300


def _get_bucket_and_common_prefix() -> tuple[str, str]:
    uri = instance_metadata("attributes/slurm_bucket_path")
    pattern = re.compile(r"gs://(?P<bucket>[^/\s]+)/(?P<path>([^/\s]+)(/[^/\s]+)*)")
    matches = pattern.match(uri)
    assert matches, f"Unexpected bucker URI: '{uri}'"
    return matches.group("bucket"), matches.group("path")


def storage_client() -> storage.Client:
    return storage.Client()


def fetch_config() -> tuple[bool, Config]:
    return None, Config(
        network_storage=[], # !!!
    )
    # bucket_name, path = _get_bucket_and_common_prefix()
    # blobs = storage_client().list_blobs(bucket_name, prefix=path)

_cfg: Config | None = None
def update_config(cfg: Config) -> None:
    global _cfg
    _cfg = cfg

def config() -> Config:
    assert _cfg, "Config is not initialized" # !!!!
    return _cfg