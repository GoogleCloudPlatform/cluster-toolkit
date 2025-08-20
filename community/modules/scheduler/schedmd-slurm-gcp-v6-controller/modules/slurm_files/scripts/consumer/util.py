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

import stat
import subprocess
import shlex
import logging
import logging.config
import logging.handlers
import sys
import functools
import requests
from pathlib import Path
import shutil
from dataclasses import dataclass
import re
from google.cloud import storage
import hashlib
import base64
import yaml


log = logging.getLogger()

SCRIPTS_DIR = Path("/slurm/scripts")
CUSTOM_SCRIPTS_DIR = Path("/slurm/custom_scripts")
LOG_DIR = Path("/var/log/slurm")

def log_subprocess(subj: subprocess.CalledProcessError | subprocess.TimeoutExpired | subprocess.CompletedProcess) -> None:
    match subj:
        case subprocess.CompletedProcess(returncode=0):
            return
        case subprocess.CompletedProcess(): # non-zero returncode
            log.error(f"Command '{subj.args}' returned exit status {subj.returncode}.")
        case subprocess.CalledProcessError() | subprocess.TimeoutExpired():
            log.error(str(subj))


def run(
    cmd:str,
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
    shell=False,
    timeout=None,
    check=True,
    universal_newlines=True,
    **kwargs,
):
    """Wrapper for subprocess.run() with convenient defaults"""
    log.debug(f"run: {cmd}")
    if not shell:
        args = shlex.split(cmd)
    else:
        args = cmd
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


def install_custom_scripts():
    """download custom scripts from gcs bucket"""
    
    if instance_role() == "compute":
        tokens = [f"nodeset-{instance_nodeset()}", "prolog", "epilog", "task_prolog", "task_epilog"]
    else:
        tokens = [f"login-{instance_login_group()}"]
    prefixes = [f"slurm-{tok}-script" for tok in tokens]

    _, common_prefix = _get_bucket_and_common_prefix()
    script_pattern = re.compile(r"^slurm-(?P<path>\S+)-script-(?P<name>\S+)")
    for blob in blob_list():
        if not any(blob.name.startswith(f"{common_prefix}/{p}") for p in prefixes):
            continue # script for some other nodeset / login group

        log.debug(f"found blob: {blob.name}")
        m = script_pattern.match(Path(blob.name).name)
        if not m:
            log.warning(f"found blob that doesn't match expected pattern: {blob.name}")
            continue

        path_parts = m["path"].split("-")
        path_parts[0] += ".d"
        stem, _, ext = m["name"].rpartition("_")
        filename = ".".join((stem, ext))

        path = Path(*path_parts, filename)
        fullpath = (CUSTOM_SCRIPTS_DIR / path).resolve()
        fullpath.parent.mkdir(parents=True, exist_ok=True)

        for par in path.parents:
            chown_slurm(CUSTOM_SCRIPTS_DIR / par)
        
        need_update = True
        if fullpath.exists():
            # TODO: MD5 reported by gcloud may differ from the one calculated here (e.g. if blob got gzipped),
            # consider using gCRC32C
            need_update = hash_file(fullpath) != blob.md5_hash

        if need_update:
            log.info(f"installing custom script: {path} from {blob.name}")
            with fullpath.open("wb") as f:
                blob.download_to_file(f)
            chown_slurm(fullpath, mode=0o755)

def hash_file(fullpath: Path) -> str:
    """Calculate MD5 hash of a file, to be comparable with GCS blob MD5"""
    with open(fullpath, "rb") as f:
        file_hash = hashlib.md5()
        chunk = f.read(8192)
        while chunk:
            file_hash.update(chunk)
            chunk = f.read(8192)
    return base64.b64encode(file_hash.digest()).decode("utf-8")


@dataclass(frozen=True)
class NSMount:
    server_ip: str
    local_mount: Path
    remote_mount: Path
    fs_type: str
    mount_options: str

    @staticmethod
    def from_dict(data: dict) -> "NSMount":
        return NSMount(
            server_ip=data["server_ip"],
            local_mount=Path(data["local_mount"]),
            remote_mount=Path(data["remote_mount"]),
            fs_type=data["fs_type"],
            mount_options=data["mount_options"],
        )


@dataclass
class Config:
    network_storage: list[NSMount]
    startup_script_timeout: int = 300

    @staticmethod
    def from_dict(data: dict) -> "Config":
        return Config(
            network_storage=[NSMount.from_dict(ns) for ns in data.get("network_storage", [])],
            startup_script_timeout=int(data.get("startup_script_timeout", 300)),
        )

def _get_bucket_and_common_prefix() -> tuple[str, str]:
    uri = instance_metadata("attributes/slurm_bucket_path")
    pattern = re.compile(r"gs://(?P<bucket>[^/\s]+)/(?P<path>([^/\s]+)(/[^/\s]+)*)")
    matches = pattern.match(uri)
    assert matches, f"Unexpected bucker URI: '{uri}'"
    return matches.group("bucket"), matches.group("path")


def storage_client() -> storage.Client:
    return storage.Client()

def blob_list(prefix="", delimiter=None):
    bucket_name, path = _get_bucket_and_common_prefix()
    blob_prefix = f"{path}/{prefix}"
    blobs = storage_client().list_blobs(
        bucket_name, prefix=blob_prefix, delimiter=delimiter
    )
    return [blob for blob in blobs]

def fetch_config() -> bool:
    config_file = SCRIPTS_DIR / "config.yaml"
    hash_file =  SCRIPTS_DIR / ".config.hash"
    old_hash = hash_file.read_text() if hash_file.exists() else None

    bucket_name, path = _get_bucket_and_common_prefix()
    if instance_role() == "compute":
        path += f"/nodeset_configs/{instance_nodeset()}.yaml"
    else:
        path += f"/login_group_configs/{instance_login_group()}.yaml"

    blob = storage_client().bucket(bucket_name).get_blob(path)  
    if blob is None:
        raise RuntimeError(f"Config file {path} does not exist")
    
    if old_hash == blob.md5_hash:
        return False
    
    blob.download_to_filename(config_file)
    chown_slurm(config_file)

    hash_file.write_text(blob.md5_hash)
    chown_slurm(hash_file)
    return True
    
_cfg: Config | None = None

def config() -> Config:
    global _cfg
    if _cfg is not None:
        return _cfg
    
    path = SCRIPTS_DIR / "config.yaml"
    if not path.exists():
        raise RuntimeError(f"Config file {path} does not exist")
    _cfg = Config.from_dict(yaml.load(path.read_text(), Loader=yaml.CLoader))
    return _cfg


def _owned_file_handler(filename):
    """special handler that will not mess up log file ownership"""
    path = Path(filename)
    if not path.exists():
        path.touch()
    chown_slurm(path)
    return logging.handlers.WatchedFileHandler(filename, delay=True)


def init_log(filename: str) -> None:
    # Hardcode DEBUG loglevel, to be restricted later
    loglevel = logging.DEBUG
    # Configure root logger
    logging.config.dictConfig({
        "version": 1,
        "disable_existing_loggers": True,
        "formatters": {
            "standard": {
                "format": "%(levelname)s: %(message)s",
            },
            "stamp": {
                "format": "%(asctime)s %(levelname)s: %(message)s",
            },
        },
        "handlers": {
            "stdout_handler": {
                "level": logging.DEBUG,
                "formatter": "standard",
                "class": "logging.StreamHandler",
                "stream": sys.stdout,
            },
            "file_handler": {
                "()": _owned_file_handler,
                "level": logging.DEBUG,
                "formatter": "stamp",
                "filename": (LOG_DIR / filename).with_suffix(".log"),
            },
        },
        "root": {
            "handlers": ["stdout_handler", "file_handler"],
            "level": loglevel,
        },
    })
