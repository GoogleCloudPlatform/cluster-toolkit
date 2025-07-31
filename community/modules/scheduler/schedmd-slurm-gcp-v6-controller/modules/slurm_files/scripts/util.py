#!/slurm/python/venv/bin/python3.13

# Copyright (C) SchedMD LLC.
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

from typing import Iterable, List, Tuple, Optional, Any, Dict, Sequence, Type, Callable, Union
import argparse
import base64
from dataclasses import dataclass, field
from datetime import timedelta, datetime, timezone
import hashlib
import inspect
import json
import logging
import logging.config
import logging.handlers
import math
import os
import re
import shlex
import shutil
import socket
import subprocess
import sys
from enum import Enum
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor, as_completed
from contextlib import contextmanager
from functools import lru_cache, reduce, wraps
from itertools import chain, islice
from pathlib import Path
from time import sleep, time

# TODO: remove "type: ignore" once moved to newer version of libraries
from google.cloud import secretmanager
from google.cloud import storage # type: ignore

import google.auth # type: ignore
from google.oauth2 import service_account # type: ignore
import googleapiclient.discovery # type: ignore
import google_auth_httplib2 # type: ignore
from googleapiclient.http import set_user_agent # type: ignore
from google.api_core.client_options import ClientOptions
import httplib2

import google.api_core.exceptions as gExceptions

import requests as requests_lib

import yaml
from addict import Dict as NSDict # type: ignore
import file_cache

USER_AGENT = "Slurm_GCP_Scripts/1.5 (GPN:SchedMD)"
ENV_CONFIG_YAML = os.getenv("SLURM_CONFIG_YAML")
if ENV_CONFIG_YAML:
    CONFIG_FILE = Path(ENV_CONFIG_YAML)
else:
    CONFIG_FILE = Path(__file__).with_name("config.yaml")
API_REQ_LIMIT = 2000


def mkdirp(path: Path) -> None:
    path.mkdir(parents=True, exist_ok=True)


scripts_dir = next(
    p for p in (Path(__file__).parent, Path("/slurm/scripts")) if p.is_dir()
)


# load all directories as Paths into a dict-like namespace
dirs = NSDict(
    home = Path("/home"),
    apps = Path("/opt/apps"),
    slurm = Path("/slurm"),
    scripts = scripts_dir,
    custom_scripts = Path("/slurm/custom_scripts"),
    munge = Path("/etc/munge"),
    secdisk = Path("/mnt/disks/sec"),
    log = Path("/var/log/slurm"),
    slurm_bucket_mount = Path("/slurm/bucket"),
)

slurmdirs = NSDict(
    prefix = Path("/usr/local"),
    etc = Path("/usr/local/etc/slurm"),
    state = Path("/var/spool/slurm"),
    key_distribution = Path("/slurm/key_distribution"),
)


# TODO: Remove this hack (relies on undocumented behavior of PyYAML)
# No need to represent NSDict and Path once we move to properly typed & serializable config.
yaml.SafeDumper.yaml_representers[
    None # type: ignore
] = lambda self, data: yaml.representer.SafeRepresenter.represent_str(self, str(data)) # type: ignore


class ApiEndpoint(Enum):
    COMPUTE = "compute"
    BQ = "bq"
    STORAGE = "storage"
    TPU = "tpu"
    SECRET = "secret_manager"


@dataclass(frozen=True)
class AcceleratorInfo:
    type: str
    count: int

    @classmethod
    def from_json(cls, jo: dict) -> "AcceleratorInfo":
        return cls(
            type=jo["guestAcceleratorType"],
            count=jo["guestAcceleratorCount"])

@dataclass(frozen=True)
class MachineType:
    name: str
    guest_cpus: int
    memory_mb: int
    accelerators: List[AcceleratorInfo]

    @classmethod
    def from_json(cls, jo: dict) -> "MachineType":
        return cls(
            name=jo["name"],
            guest_cpus=jo["guestCpus"],
            memory_mb=jo["memoryMb"],
            accelerators=[
                AcceleratorInfo.from_json(a) for a in jo.get("accelerators", [])],
        )

    @property
    def family(self) -> str:
        # TODO: doesn't work with N1 custom machine types
        # See https://cloud.google.com/compute/docs/instances/creating-instance-with-custom-machine-type#create
        return self.name.split("-")[0]

    @property
    def supports_smt(self) -> bool:
        # https://cloud.google.com/compute/docs/cpu-platforms
        if self.family in  ("t2a", "t2d", "h3", "c4a", "h4d",):
            return False
        if self.guest_cpus == 1:
            return False
        return True

    @property
    def sockets(self) -> int:
        return {
            "h3": 2,
            "h4d": 2,
            "c2d": 2 if self.guest_cpus > 56 else 1,
            "a3": 2,
            "c2": 2 if self.guest_cpus > 30 else 1,
            "c3": 2 if self.guest_cpus > 88 else 1,
            "c3d": 2 if self.guest_cpus > 180 else 1,
            "c4": 2 if self.guest_cpus > 96 else 1,
            "c4d": 2 if self.guest_cpus > 192 else 1,
        }.get(
            self.family,
            1,  # assume 1 socket for all other families
        )


@dataclass(frozen=True)
class UpcomingMaintenance:
    window_start_time: datetime

    @classmethod
    def from_json(cls, jo: Optional[dict]) -> Optional["UpcomingMaintenance"]:
        if jo is None:
            return None
        try:
            if "windowStartTime" in jo:
                ts = parse_gcp_timestamp(jo["windowStartTime"])
            elif "startTimeWindow" in jo:
                ts = parse_gcp_timestamp(jo["startTimeWindow"]["earliest"])
            else:
                raise Exception("Neither windowStartTime nor startTimeWindow are found")
        except BaseException as e:
            raise ValueError(f"Unexpected format for upcomingMaintenance: {jo}") from e
        return cls(window_start_time=ts)

@dataclass(frozen=True)
class InstanceResourceStatus:
    physical_host: Optional[str]
    upcoming_maintenance: Optional[UpcomingMaintenance]

    @classmethod
    def from_json(cls, jo: Optional[dict]) -> "InstanceResourceStatus":
        if not jo:
            return cls(
                physical_host=None,
                upcoming_maintenance=None,
            )

        try:
            maint = UpcomingMaintenance.from_json(jo.get("upcomingMaintenance"))
        except ValueError as e:
            log.exception("Failed to parse upcomingMaintenance, ignoring")
            maint = None # intentionally swallow exception

        return cls(
            physical_host=jo.get("physicalHost"),
            upcoming_maintenance=maint,
        )


@dataclass(frozen=True)
class Instance:
  name: str
  zone: str
  status: str
  creation_timestamp: datetime
  role: Optional[str]
  resource_status: InstanceResourceStatus
  metadata: Dict[str, str]
  # TODO: use proper InstanceScheduling class
  scheduling: NSDict

  @classmethod
  def from_json(cls, jo: dict) -> "Instance":
    return cls(
      name=jo["name"],
      zone=trim_self_link(jo["zone"]),
      status=jo["status"],
      creation_timestamp=parse_gcp_timestamp(jo["creationTimestamp"]),
      resource_status=InstanceResourceStatus.from_json(jo.get("resourceStatus")),
      scheduling=NSDict(jo.get("scheduling")),
      role = jo.get("labels", {}).get("slurm_instance_role"),
      metadata = {k["key"]: k["value"] for k in jo.get("metadata", {}).get("items", [])}
    )


@dataclass(frozen=True)
class NSMount:
    server_ip: str
    local_mount: Path
    remote_mount: Path
    fs_type: str
    mount_options: str

@lru_cache(maxsize=1)
def default_credentials():
    return google.auth.default()[0]


@lru_cache(maxsize=1)
def authentication_project():
    return google.auth.default()[1]


DEFAULT_UNIVERSE_DOMAIN = "googleapis.com"


def now() -> datetime:
    """
    Return current time as timezone-aware datetime.

    IMPORTANT: DO NOT use `datetime.now()`, unless you explicitly need to have tz-naive datetime.
    Otherwise there is a risk of getting: "cannot compare naive and aware datetimes" error,
    since all timetstamps we receive from GCP API are tz-aware.

    Another motivation  for this function is to allow to mock time in tests.
    """
    return datetime.now(timezone.utc)

def parse_gcp_timestamp(s: str) -> datetime:
  """
  Parse timestamp strings returned by GCP API into datetime.
  Works with both Zulu and non-Zulu timestamps.
  NOTE: It always return tz-aware datetime (fallbacks to UTC and logs error).
  """
  # Requires Python >= 3.7
  # TODO: Remove this "hack" of trimming the Z from timestamps once we move to Python 3.11
  # (context: https://discuss.python.org/t/parse-z-timezone-suffix-in-datetime/2220/30)
  ts = datetime.fromisoformat(s.replace('Z', '+00:00'))
  if ts.tzinfo is None: # fallback to UTC
    log.error(f"Received timestamp without timezone info: {s}")
    ts = ts.replace(tzinfo=timezone.utc)
  return ts


def universe_domain() -> str:
    try:
        return instance_metadata("attributes/universe_domain")
    except MetadataNotFoundError:
        return DEFAULT_UNIVERSE_DOMAIN


def endpoint_version(api: ApiEndpoint) -> Optional[str]:
    return lookup().endpoint_versions.get(api.value, None)


@lru_cache(maxsize=1)
def get_credentials() -> Optional[service_account.Credentials]:
    """Get credentials for service account"""
    key_path = os.environ.get("GOOGLE_APPLICATION_CREDENTIALS")
    if key_path is not None:
        credentials = service_account.Credentials.from_service_account_file(
            key_path, scopes=[f"https://www.{universe_domain()}/auth/cloud-platform"]
        )
    else:
        credentials = default_credentials()

    return credentials


@lru_cache(maxsize=1)
def get_dev_key() -> Optional[str]:
    """Get dev key for project (uses json or yaml format)"""
    try:
        with open("/etc/slurm/slurm_vars.yaml", 'r') as file:
            data = yaml.safe_load(file)
            return data['google_developer_key']
    except:
        return None


def create_client_options(api: ApiEndpoint) -> ClientOptions:
    """Create client options for cloud endpoints"""
    ver = endpoint_version(api)
    ud = universe_domain()
    options = {}
    if ud and ud != DEFAULT_UNIVERSE_DOMAIN:
        options["universe_domain"] = ud
    if ver:
        options["api_endpoint"] = f"https://{api.value}.{ud}/{ver}/"
    co = ClientOptions(**options)
    log.debug(f"Using ClientOptions = {co} for API: {api.value}")
    return co

log = logging.getLogger()


def access_secret_version(project_id, secret_id, version_id="latest"):
    """
    Access the payload for the given secret version if one exists. The version
    can be a version number as a string (e.g. "5") or an alias (e.g. "latest").
    """
    co = create_client_options(ApiEndpoint.SECRET)
    client = secretmanager.SecretManagerServiceClient(client_options=co)
    name = f"projects/{project_id}/secrets/{secret_id}/versions/{version_id}"
    try:
        response = client.access_secret_version(request={"name": name})
        log.debug(f"Secret '{name}' was found.")
        payload = response.payload.data.decode("UTF-8")
    except gExceptions.NotFound:
        log.debug(f"Secret '{name}' was not found!")
        payload = None

    return payload


def parse_self_link(self_link: str):
    """Parse a selfLink url, extracting all useful values
    https://.../v1/projects/<project>/regions/<region>/...
    {'project': <project>, 'region': <region>, ...}
    can also extract zone, instance (name), image, etc
    """
    link_patt = re.compile(r"(?P<key>[^\/\s]+)s\/(?P<value>[^\s\/]+)")
    return NSDict(link_patt.findall(self_link))


def parse_bucket_uri(uri: str):
    """
    Parse a bucket url
    E.g. gs://<bucket_name>/<path>
    """
    pattern = re.compile(r"gs://(?P<bucket>[^/\s]+)/(?P<path>([^/\s]+)(/[^/\s]+)*)")
    matches = pattern.match(uri)
    assert matches, f"Unexpected bucker URI: '{uri}'"
    return matches.group("bucket"), matches.group("path")


def trim_self_link(link: str):
    """get resource name from self link url, eg.
    https://.../v1/projects/<project>/regions/<region>
    -> <region>
    """
    try:
        return link[link.rindex("/") + 1 :]
    except ValueError:
        raise Exception(f"'/' not found, not a self link: '{link}' ")


def execute_with_futures(func, seq):
    with ThreadPoolExecutor() as exe:
        futures = []
        for i in seq:
            future = exe.submit(func, i)
            futures.append(future)
        for future in as_completed(futures):
            result = future.exception()
            if result is not None:
                raise result


def map_with_futures(func, seq):
    with ThreadPoolExecutor() as exe:
        futures = []
        for i in seq:
            future = exe.submit(func, i)
            futures.append(future)
        for future in futures:
            # Will be result or raise Exception
            res = None
            try:
                res = future.result()
            except Exception as e:
                res = e
            yield res

def should_mount_slurm_bucket() -> bool:
    try:
        return instance_metadata("attributes/slurm_bucket_mount", silent=True).lower() == "true"
    except MetadataNotFoundError:
        return False


def _get_bucket_and_common_prefix() -> Tuple[str, str]:
    uri = instance_metadata("attributes/slurm_bucket_path")
    return parse_bucket_uri(uri)

def blob_get(file):
    bucket_name, path = _get_bucket_and_common_prefix()
    blob_name = f"{path}/{file}"
    return storage_client().get_bucket(bucket_name).blob(blob_name)


def blob_list(prefix="", delimiter=None):
    bucket_name, path = _get_bucket_and_common_prefix()
    blob_prefix = f"{path}/{prefix}"
    # Note: The call returns a response only when the iterator is consumed.
    blobs = storage_client().list_blobs(
        bucket_name, prefix=blob_prefix, delimiter=delimiter
    )
    return [blob for blob in blobs]

def file_list(prefix="", subpath="") -> List[os.DirEntry]:
    path = dirs.slurm_bucket_mount
    file_prefix = f"{path}/{subpath}"
    try:
        files = os.scandir(file_prefix)
        return [file for file in files if file.name.startswith(prefix)]
    except:
        return [] 
     # Not considering lack of file's existence as fatal (we may check for files we know don't exist).
     # Responsibility of callee to determine if it is fatal or not, blob_list returns empty iterator in similar cases.

def hash_file(fullpath: Path) -> str:
    with open(fullpath, "rb") as f:
        file_hash = hashlib.md5()
        chunk = f.read(8192)
        while chunk:
            file_hash.update(chunk)
            chunk = f.read(8192)
    return base64.b64encode(file_hash.digest()).decode("utf-8")


def install_custom_scripts(check_hash:bool=False):
    """download custom scripts from gcs bucket"""
    role, tokens = lookup().instance_role, []

    mounted_scripts=False
    if should_mount_slurm_bucket() and role != "controller":
        mounted_scripts=True

    all_prolog_tokens = ["prolog", "epilog", "task_prolog", "task_epilog"]
    if role == "controller":
        tokens = ["controller"] + all_prolog_tokens
    elif role == "compute":
        tokens = [f"nodeset-{lookup().node_nodeset_name()}"] + all_prolog_tokens
    elif role == "login":
        tokens = [f"login-{instance_login_group()}"]

    prefixes = [f"slurm-{tok}-script" for tok in tokens]

    # TODO: use single `blob_list`, to reduce ~4x number of GCS requests
    if mounted_scripts:
        source_collection = list(chain.from_iterable(file_list(prefix=p) for p in prefixes))
    else:
        source_collection = list(chain.from_iterable(blob_list(prefix=p) for p in prefixes))

    script_pattern = re.compile(r"^slurm-(?P<path>\S+)-script-(?P<name>\S+)")
    for source in source_collection:
        if mounted_scripts:
            m = script_pattern.match(source.name)
        else:
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

        if check_hash and fullpath.exists() and isinstance(source,storage.Blob):
            # TODO: MD5 reported by gcloud may differ from the one calculated here (e.g. if blob got gzipped),
            # consider using gCRC32C
            need_update = hash_file(fullpath) != source.md5_hash

        log.info(f"installing custom script: {path} from {source.name}")

        if isinstance(source,os.DirEntry):
            shutil.copy(source.path, fullpath) #Needs to be copied since mounted nfs is read-only
            chown_slurm(fullpath, mode=0o755)

        elif need_update:
            with fullpath.open("wb") as f:
                source.download_to_file(f)
            chown_slurm(fullpath, mode=0o755)

def compute_service(version="beta"):
    """Make thread-safe compute service handle
    creates a new Http for each request
    """
    credentials = get_credentials()
    dev_key = get_dev_key()

    def build_request(http, *args, **kwargs):
        new_http = set_user_agent(httplib2.Http(), USER_AGENT)
        if credentials is not None:
            new_http = google_auth_httplib2.AuthorizedHttp(credentials, http=new_http)
        return googleapiclient.http.HttpRequest(new_http, *args, **kwargs)

    ver = endpoint_version(ApiEndpoint.COMPUTE)
    disc_url = googleapiclient.discovery.DISCOVERY_URI
    if ver:
        version = ver
        disc_url = disc_url.replace(DEFAULT_UNIVERSE_DOMAIN, universe_domain())

    log.debug(f"Using version={version} of Google Compute Engine API")
    return googleapiclient.discovery.build(
        "compute",
        version,
        requestBuilder=build_request,
        credentials=credentials,
        developerKey=dev_key,
        discoveryServiceUrl=disc_url,
        cache_discovery=False, # See https://github.com/googleapis/google-api-python-client/issues/299
    )

def storage_client() -> storage.Client:
    """
    Config-independent storage client
    """
    ud = universe_domain()
    co = {}
    if ud and ud != DEFAULT_UNIVERSE_DOMAIN:
        co["universe_domain"] = ud
    return storage.Client(client_options=ClientOptions(**co))


class DeffetiveStoredConfigError(Exception):
    """
    Raised when config can not be loaded and assembled from bucket
    """
    pass


def _fill_cfg_defaults(cfg: NSDict) -> NSDict:
    if not cfg.slurm_log_dir:
        cfg.slurm_log_dir = dirs.log
    if not cfg.slurm_bin_dir:
        cfg.slurm_bin_dir = slurmdirs.prefix / "bin"
    if not cfg.slurm_control_hosts:
        raise DeffetiveStoredConfigError("No controllers defined in config")
    if not cfg.slurm_control_host_port:
        cfg.slurm_control_host_port = "6820-6830"
    return cfg

@dataclass
class _ConfigBlobs:
    """
    "Private" class that represent a collection of GCS blobs for configuration
    """
    core: storage.Blob
    controller_addr: Optional[storage.Blob]
    partition: List[storage.Blob] = field(default_factory=list)
    nodeset: List[storage.Blob] = field(default_factory=list)
    nodeset_dyn: List[storage.Blob] = field(default_factory=list)
    nodeset_tpu: List[storage.Blob] = field(default_factory=list)
    login_group: List[storage.Blob] = field(default_factory=list)

    @property
    def hash(self) -> str:
        h = hashlib.md5()
        all = [self.core] + self.partition + self.nodeset + self.nodeset_dyn + self.nodeset_tpu
        if self.controller_addr:
            all.append(self.controller_addr)
    
        # sort blobs so hash is consistent
        for blob in sorted(all, key=lambda b: b.name):
            h.update(blob.md5_hash.encode("utf-8"))
        return h.hexdigest()

@dataclass
class _ConfigFiles:
    """
    "Private" class that represent a collection of files for configuration
    """
    core: Path
    controller_addr: Optional[Path]
    partition: List[Path] = field(default_factory=list)
    nodeset: List[Path] = field(default_factory=list)
    nodeset_dyn: List[Path] = field(default_factory=list)
    nodeset_tpu: List[Path] = field(default_factory=list)
    login_group: List[Path] = field(default_factory=list)

def _list_config_blobs() -> _ConfigBlobs:
    _, common_prefix = _get_bucket_and_common_prefix()

    core: Optional[storage.Blob] = None
    controller_addr: Optional[storage.Blob] = None
    rest: Dict[str, List[storage.Blob]] = {"partition": [], "nodeset": [], "nodeset_dyn": [], "nodeset_tpu": [], "login_group": []}

    is_controller = instance_role() == "controller"

    for blob in blob_list(prefix=""):
        if blob.name == f"{common_prefix}/config.yaml":
            core = blob
        if blob.name == f"{common_prefix}/controller_addr.yaml" and not is_controller:
            # Don't add this config blobs for controller to avoid "double reconfiguration":
            # Initially this file doesn't exist and produce later by `setup_controller`;
            # Appearance of this blob would trigger change in combined hash of config files;
            # Ignore existence of this file for controller, assume that
            # no other instance nodes will proceed with configuration until this file is created.
            controller_addr = blob
        for key in rest.keys():
            if blob.name.startswith(f"{common_prefix}/{key}_configs/"):
                rest[key].append(blob)

    if core is None:
        raise DeffetiveStoredConfigError(f"{common_prefix}/config.yaml not found in bucket")
    
    return _ConfigBlobs(core=core, controller_addr=controller_addr, **rest)

def _list_config_files() -> _ConfigFiles:
    file_dir = dirs.slurm_bucket_mount
    core: Optional[Path] = None
    controller_addr: Optional[Path] = None
    rest: Dict[str, List[Path]] = {"partition": [], "nodeset": [], "nodeset_dyn": [], "nodeset_tpu": [], "login_group": []}

    if Path(f"{file_dir}/config.yaml").exists():
        core = Path(f"{file_dir}/config.yaml")

    for key in rest.keys():
        for f in file_list(subpath=f"{key}_configs"):
            rest[key].append(f.path)

    if core is None:
        raise Exception(f"config.yaml was not found in mounted folder: {dirs.slurm_bucket_mount}") #Intentionally not using DeffetiveStoredConfigError as this is considered a fatal error
    
    return _ConfigFiles(core=core, controller_addr=None, **rest)

def _fetch_config(old_hash: Optional[str]) -> Optional[Tuple[NSDict, str]]:
    """Fetch config from bucket, returns None if no changes are detected."""
    blobs = _list_config_blobs()
    if old_hash == blobs.hash:
        return None

    def _download(bs) -> List[Any]:
        return [yaml.safe_load(b.download_as_text()) for b in bs]

    return _assemble_config(
        core=_download([blobs.core])[0],
        controller_addr=_download([blobs.controller_addr])[0] if blobs.controller_addr else None,
        partitions=_download(blobs.partition),
        nodesets=_download(blobs.nodeset),
        nodesets_dyn=_download(blobs.nodeset_dyn),
        nodesets_tpu=_download(blobs.nodeset_tpu),
        login_groups=_download(blobs.login_group),
    ), blobs.hash

def _fetch_mounted_config() -> Optional[Tuple[NSDict, str]]:
    if not dirs.slurm_bucket_mount.is_mount():
        raise Exception(f"{dirs.slurm_bucket_mount} is not mounted")

    files = _list_config_files()

    def _load(files) -> List[Any]:
        file_yaml=[]
        for file in files:
            with open(file, "r") as f:
                file_yaml.append(yaml.safe_load(f))
        return file_yaml

    return _assemble_config(
        core=_load([files.core])[0],
        controller_addr=None,
        partitions=_load(files.partition),
        nodesets=_load(files.nodeset),
        nodesets_dyn=_load(files.nodeset_dyn),
        nodesets_tpu=_load(files.nodeset_tpu),
        login_groups=_load(files.login_group),
    )

def controller_lookup_self_ip() -> str:
    assert instance_role() == "controller"
    # Get IP of LAST network-interface
    # TODO: Consider change order of NICs definition, so right NIC is always @0.
    idx = instance_metadata("network-interfaces").split()[-1] # either `0/` or `1/`
    return instance_metadata(f"network-interfaces/{idx}ip")

def _assemble_config(
        core: Any,
        controller_addr: Optional[Any],
        partitions: List[Any],
        nodesets: List[Any],
        nodesets_dyn: List[Any],
        nodesets_tpu: List[Any],
        login_groups: List[Any],
    ) -> NSDict:
    cfg = NSDict(core)

    if cfg.controller_network_attachment:
        # lookup controller address
        if instance_role() == "controller":
            # ignore stored value of `controller_addr`, it will be overwritten during `setup_controller`
            cfg.slurm_control_addr = controller_lookup_self_ip()
        else:   
            if not controller_addr: 
                raise DeffetiveStoredConfigError("controller_addr.yaml not found in bucket")
            cfg.slurm_control_addr = controller_addr["slurm_control_addr"]

    # add partition configs
    for p_yaml in partitions:
        p_cfg = NSDict(p_yaml)
        assert p_cfg.get("partition_name"), "partition_name is required"
        p_name = p_cfg.partition_name
        assert p_name not in cfg.partitions, f"partition {p_name} already defined"
        cfg.partitions[p_name] = p_cfg

    # add nodeset configs
    ns_names = set()
    def _add_nodesets(yamls: List[Any], target: dict):
        for ns_yaml in yamls:
            ns_cfg = NSDict(ns_yaml)
            assert ns_cfg.get("nodeset_name"), "nodeset_name is required"
            ns_name = ns_cfg.nodeset_name
            assert ns_name not in ns_names, f"nodeset {ns_name} already defined"
            target[ns_name] = ns_cfg
            ns_names.add(ns_name)

    _add_nodesets(nodesets, cfg.nodeset)
    _add_nodesets(nodesets_dyn, cfg.nodeset_dyn)
    _add_nodesets(nodesets_tpu, cfg.nodeset_tpu)

    # validate that configs for all referenced nodesets are present
    for p in cfg.partitions.values():
        for ns_name in chain(p.partition_nodeset, p.partition_nodeset_dyn, p.partition_nodeset_tpu):
            if ns_name not in ns_names:
                raise DeffetiveStoredConfigError(f"nodeset {ns_name} not defined in config")
            
    for lg_yaml in login_groups:
        lg_cfg = NSDict(lg_yaml)
        assert lg_cfg.get("group_name"), "group_name is required"
        lg_name = lg_cfg.group_name
        assert lg_name not in cfg.login_groups
        cfg.login_groups[lg_name] = lg_cfg

    if instance_role() == "login":
        group = instance_login_group()
        if group not in cfg.login_groups:
            raise DeffetiveStoredConfigError(f"login group '{group}' does not exist in config")

    return _fill_cfg_defaults(cfg)

def fetch_config() -> Tuple[bool, NSDict]:
    """
    Fetches config from bucket and saves it locally
    Returns True if new (updated) config was fetched
    """
    hash_file = Path("/slurm/scripts/.config.hash")
    old_hash = hash_file.read_text() if hash_file.exists() else None
    
    if should_mount_slurm_bucket() and instance_role() != "controller":
        cfg = _fetch_mounted_config()
        CONFIG_FILE.write_text(yaml.dump(cfg, Dumper=Dumper))
        chown_slurm(CONFIG_FILE)
        return False, cfg
    
    cfg_and_hash = _fetch_config(old_hash=old_hash)
    
    if not cfg_and_hash:
        return False, _load_config()

    cfg, hash = cfg_and_hash
    hash_file.write_text(hash)
    chown_slurm(hash_file)
    CONFIG_FILE.write_text(yaml.dump(cfg, Dumper=Dumper))
    chown_slurm(CONFIG_FILE)
    return True, cfg

def owned_file_handler(filename):
    """create file handler"""
    chown_slurm(filename)
    return logging.handlers.WatchedFileHandler(filename, delay=True)

def get_log_path() -> Path:
    """
    Returns path to log file for the current script.
    e.g. resume.py -> /var/log/slurm/resume.log
    """
    cfg_log_dir = lookup().cfg.slurm_log_dir
    log_dir = Path(cfg_log_dir) if cfg_log_dir else dirs.log
    return (log_dir / Path(sys.argv[0]).name).with_suffix(".log")

def init_log_and_parse(parser: argparse.ArgumentParser) -> argparse.Namespace:
    parser.add_argument(
        "--debug",
        "-d",
        dest="loglevel",
        action="store_const",
        const=logging.DEBUG,
        default=logging.INFO,
        help="Enable debugging output",
    )
    parser.add_argument(
        "--trace-api",
        "-t",
        action="store_true",
        help="Enable detailed api request output",
    )
    args = parser.parse_args()
    loglevel = args.loglevel
    if lookup().cfg.enable_debug_logging:
        loglevel = logging.DEBUG
    if args.trace_api:
        lookup().cfg.extra_logging_flags["trace_api"] = True
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
                "()": owned_file_handler,
                "level": logging.DEBUG,
                "formatter": "stamp",
                "filename": get_log_path(),
            },
        },
        "root": {
            "handlers": ["stdout_handler", "file_handler"],
            "level": loglevel,
        },
    })

    sys.excepthook = _handle_exception

    return args


def log_api_request(request):
    """log.trace info about a compute API request"""
    if not lookup().cfg.extra_logging_flags.get("trace_api"):
        return
    # output the whole request object as pretty yaml
    # the body is nested json, so load it as well
    rep = json.loads(request.to_json())
    if rep.get("body", None) is not None:
        rep["body"] = json.loads(rep["body"])
    pretty_req = yaml.safe_dump(rep).rstrip()
    # label log message with the calling function
    log.debug(f"{inspect.stack()[1].function}:\n{pretty_req}")


def _handle_exception(exc_type, exc_value, exc_trace):
    """log exceptions other than KeyboardInterrupt"""
    if not issubclass(exc_type, KeyboardInterrupt):
        log.exception("Fatal exception", exc_info=(exc_type, exc_value, exc_trace))
    sys.__excepthook__(exc_type, exc_value, exc_trace)


def run(
    args,
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
    shell=False,
    timeout=None,
    check=True,
    universal_newlines=True,
    **kwargs,
):
    """Wrapper for subprocess.run() with convenient defaults"""
    if isinstance(args, list):
        args = list(filter(lambda x: x is not None, args))
        args = " ".join(args)
    if not shell and isinstance(args, str):
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
        

    def normalize(out: None | str | bytes) -> None | str:
        """
        Turns stderr and stdout into string:
        > A bytes sequence, or a string if run() was called with an encoding, errors, or text=True. None if was not captured.
        """
        match out:
            case None:
                return None
            case str():
                return out.strip()
            case bytes():
                return out.decode().strip()
            case _:
                return repr(out)

    if stdout := normalize(subj.stdout):
        log.error(f"stdout: {stdout}")
    if stderr := normalize(subj.stderr):
        log.error(f"stderr: {stderr}")


def chown_slurm(path: Path, mode=None) -> None:
    if path.exists():
        if mode:
            path.chmod(mode)
    else:
        mkdirp(path.parent)
        if mode:
            path.touch(mode=mode)
        else:
            path.touch()
    try:
        shutil.chown(path, user="slurm", group="slurm")
    except LookupError:
        log.warning(f"User 'slurm' does not exist. Cannot 'chown slurm:slurm {path}'.")
    except PermissionError:
        log.warning(f"Not authorized to 'chown slurm:slurm {path}'.")
    except Exception as err:
        log.error(err)


@contextmanager
def cd(path):
    """Change working directory for context"""
    prev = Path.cwd()
    os.chdir(path)
    try:
        yield
    finally:
        os.chdir(prev)


def cached_property(f):
    return property(lru_cache()(f))


def retry(max_retries: int, init_wait_time: float, warn_msg: str, exc_type: Type[Exception]):
    """Retries functions that raises the exception exc_type.
    Retry time is increased by a factor of two for every iteration.

    Args:
        max_retries (int): Maximum number of retries
        init_wait_time (float): Initial wait time in secs
        warn_msg (str): Message to print during retries
        exc_type (Exception): Exception type to check for
    """

    if max_retries <= 0:
        raise ValueError("Incorrect value for max_retries, must be >= 1")
    if init_wait_time <= 0.0:
        raise ValueError("Invalid value for init_wait_time, must be > 0.0")

    def decorator(f):
        @wraps(f)
        def wrapper(*args, **kwargs):
            retry = 0
            secs = init_wait_time
            captured_exc: Optional[BaseException] = None
            while retry < max_retries:
                try:
                    return f(*args, **kwargs)
                except exc_type as e:
                    captured_exc = e
                    log.warn(f"{warn_msg}, retrying in {secs}")
                    sleep(secs)
                    retry += 1
                    secs *= 2
            assert captured_exc
            raise captured_exc

        return wrapper

    return decorator


def separate(pred: Callable[[Any], bool], coll: Iterable[Any]) -> Tuple[List[Any], List[Any]]:
    """filter into 2 lists based on pred returning True or False
    returns ([False], [True])
    """
    res: Tuple[List[Any], List[Any]] = ([],[])
    for el in coll:
        res[pred(el)].append(el)
    return res


def chunked(iterable, n=API_REQ_LIMIT):
    """group iterator into chunks of max size n"""
    it = iter(iterable)
    while True:
        chunk = list(islice(it, n))
        if not chunk:
            return
        yield chunk

def groupby_unsorted(seq: Sequence[Any], key):
    indices = defaultdict(list)
    for i, el in enumerate(seq):
        indices[key(el)].append(i)
    for k, idxs in indices.items():
        yield k, (seq[i] for i in idxs)


@lru_cache(maxsize=32)
def find_ratio(a, n, s, r0=None):
    """given the start (a), count (n), and sum (s), find the ratio required"""
    if n == 2:
        return s / a - 1
    an = a * n
    if n == 1 or s == an:
        return 1
    if r0 is None:
        # we only need to know which side of 1 to guess, and the iteration will work
        r0 = 1.1 if an < s else 0.9

    # geometric sum formula
    def f(r):
        return a * (1 - r**n) / (1 - r) - s

    # derivative of f
    def df(r):
        rm1 = r - 1
        rn = r**n
        return (a * (rn * (n * rm1 - r) + r)) / (r * rm1**2)

    MIN_DR = 0.0001  # negligible change
    r = r0
    # print(f"r(0)={r0}")
    MAX_TRIES = 64
    for i in range(1, MAX_TRIES + 1):
        try:
            dr = f(r) / df(r)
        except ZeroDivisionError:
            log.error(f"Failed to find ratio due to zero division! Returning r={r0}")
            return r0
        r = r - dr
        # print(f"r({i})={r}")
        # if the change in r is small, we are close enough
        if abs(dr) < MIN_DR:
            break
    else:
        log.error(f"Could not find ratio after {MAX_TRIES}! Returning r={r0}")
        return r0
    return r


def backoff_delay(start, timeout=None, ratio=None, count: int = 0):
    """generates `count` waits starting at `start`
    sum of waits is `timeout` or each one is `ratio` bigger than the last
    the last wait is always 0"""
    # timeout or ratio must be set but not both
    assert (timeout is None) ^ (ratio is None)
    assert ratio is None or ratio > 0
    assert timeout is None or timeout >= start
    assert (count > 1 or timeout is not None) and isinstance(count, int)
    assert start > 0

    if count == 0:
        # Equation for auto-count is tuned to have a max of
        # ~int(timeout) counts with a start wait of <0.01.
        # Increasing start wait decreases count eg.
        # backoff_delay(10, timeout=60) -> count = 5
        count = int(
            (timeout / ((start + 0.05) ** (1 / 2)) + 2) // math.log(timeout + 2)
        )

    yield start
    # if ratio is set:
    # timeout = start * (1 - ratio**(count - 1)) / (1 - ratio)
    if ratio is None:
        ratio = find_ratio(start, count - 1, timeout)

    wait = start
    # we have start and 0, so we only need to generate count - 2
    for _ in range(count - 2):
        wait *= ratio
        yield wait
    yield 0
    return


ROOT_URL = "http://metadata.google.internal/computeMetadata/v1"

class MetadataNotFoundError(Exception):
    pass

def get_metadata(path:str, silent=False) -> str:
    """Get metadata relative to metadata/computeMetadata/v1"""
    HEADERS = {"Metadata-Flavor": "Google"}
    url = f"{ROOT_URL}/{path}"
    try:
        resp = requests_lib.get(url, headers=HEADERS)
        resp.raise_for_status()
        return resp.text
    except requests_lib.exceptions.HTTPError:
        if not silent:
            log.warning(f"metadata not found ({url})")
        raise MetadataNotFoundError(f"failed to get_metadata from {url}")


@lru_cache(maxsize=None)
def instance_metadata(path: str, silent:bool=False) -> str:
    return get_metadata(f"instance/{path}", silent=silent)

def instance_role():
    return instance_metadata("attributes/slurm_instance_role")


def instance_login_group():
    return instance_metadata("attributes/slurm_login_group")


def natural_sort(text):
    def atoi(text):
        return int(text) if text.isdigit() else text

    return [atoi(w) for w in re.split(r"(\d+)", text)]


def to_hostlist(names: Iterable[str]) -> str:
    """
    Fast implementation of `hostlist` that doesn't invoke `scontrol`
    IMPORTANT:
    * Acts as `scontrol show hostlistsorted`, i.e. original order is not preserved
    * Achieves worse compression than `scontrol show hostlist` for some cases
    """
    pref = defaultdict(list)
    tokenizer = re.compile(r"^(.*?)(\d*)$")
    for name in filter(None, names):
        matches = tokenizer.match(name)
        assert matches, name
        p, s = matches.groups()
        pref[p].append(s)

    def _compress_suffixes(ss: List[str]) -> List[str]:
        cur, res = None, []

        def cur_repr():
            assert cur
            nums, strs = cur
            if nums[0] == nums[1]:
                return strs[0]
            return f"{strs[0]}-{strs[1]}"

        for s in sorted(ss, key=int):
            n = int(s)
            if cur is None:
                cur = ((n, n), (s, s))
                continue

            nums, strs = cur
            if n == nums[1] + 1:
                cur = ((nums[0], n), (strs[0], s))
            else:
                res.append(cur_repr())
                cur = ((n, n), (s, s))
        if cur:
            res.append(cur_repr())
        return res

    res = []
    for p in sorted(pref.keys()):
        sl = defaultdict(list)
        for s in pref[p]:
            sl[len(s)].append(s)
        cs = []
        for ln in sorted(sl.keys()):
            if ln == 0:
                res.append(p)
            else:
                cs.extend(_compress_suffixes(sl[ln]))
        if not cs:
            continue
        if len(cs) == 1 and "-" not in cs[0]:
            res.append(f"{p}{cs[0]}")
        else:
            res.append(f"{p}[{','.join(cs)}]")
    return ",".join(res)

@lru_cache(maxsize=None)
def to_hostnames(nodelist: str) -> List[str]:
    """make list of hostnames from hostlist expression"""
    if not nodelist:
        return []  # avoid degenerate invocation of scontrol
    if isinstance(nodelist, str):
        hostlist = nodelist
    else:
        hostlist = ",".join(nodelist)
    hostnames = run(f"{lookup().scontrol} show hostnames {hostlist}").stdout.splitlines()
    return hostnames


def retry_exception(exc) -> bool:
    """return true for exceptions that should always be retried"""
    msg = str(exc)
    retry_errors = (
        "Rate Limit Exceeded",
        "Quota Exceeded",
        "Quota exceeded",
    )
    return any(err in msg for err in retry_errors)


def ensure_execute(request):
    """Handle rate limits and socket time outs"""

    for retry, wait in enumerate(backoff_delay(0.5, timeout=10 * 60, count=20)):
        try:
            return request.execute()
        except googleapiclient.errors.HttpError as e:
            if retry_exception(e):
                log.error(f"retry:{retry} '{e}'")
                sleep(wait)
                continue
            raise

        except socket.timeout as e:
            # socket timed out, try again
            log.debug(e)

        except Exception as e:
            log.error(e, exc_info=True)
            raise

        break


def batch_execute(requests, retry_cb=None, log_err=log.error):
    """execute list or dict<req_id, request> as batch requests
    retry if retry_cb returns true
    """
    BATCH_LIMIT = 1000
    if not isinstance(requests, dict):
        requests = {str(k): v for k, v in enumerate(requests)}  # rid generated here
    done = {}
    failed = {}
    timestamps: List[float] = []
    rate_limited = False

    def batch_callback(rid, resp, exc):
        nonlocal rate_limited
        if exc is not None:
            log_err(f"compute request exception {rid}: {exc}")
            if retry_exception(exc):
                rate_limited = True
            else:
                req = requests.pop(rid)
                failed[rid] = (req, exc)
        else:
            # if retry_cb is set, don't move to done until it returns false
            if retry_cb is None or not retry_cb(resp):
                requests.pop(rid)
                done[rid] = resp

    def batch_request(reqs):
        batch = lookup().compute.new_batch_http_request(callback=batch_callback)
        for rid, req in reqs:
            batch.add(req, request_id=rid)
        return batch

    while requests:
        if timestamps:
            timestamps = [stamp for stamp in timestamps if stamp > time()]
        if rate_limited and timestamps:
            stamp = next(iter(timestamps))
            sleep(max(stamp - time(), 0))
            rate_limited = False
        # up to API_REQ_LIMIT (2000) requests
        # in chunks of up to BATCH_LIMIT (1000)
        batches = [
            batch_request(chunk)
            for chunk in chunked(islice(requests.items(), API_REQ_LIMIT), BATCH_LIMIT)
        ]
        timestamps.append(time() + 100)
        with ThreadPoolExecutor() as exe:
            futures = []
            for batch in batches:
                future = exe.submit(ensure_execute, batch)
                futures.append(future)
            for future in futures:
                result = future.exception()
                if result is not None:
                    raise result

    return done, failed


def get_operation_req(lkp: "Lookup", name: str, region: Optional[str]=None, zone: Optional[str]=None) -> Any:
  if zone:
    return lkp.compute.zoneOperations().get(project=lkp.project, zone=zone, operation=name)
  elif region:
    return lkp.compute.regionOperations().get(project=lkp.project, region=region, operation=name)
  return lkp.compute.globalOperations().get(project=lkp.project, operation=name)

def wait_request(operation, project: str):
    """makes the appropriate wait request for a given operation"""
    if "zone" in operation:
        req = lookup().compute.zoneOperations().wait(
            project=project,
            zone=trim_self_link(operation["zone"]),
            operation=operation["name"],
        )
    elif "region" in operation:
        req = lookup().compute.regionOperations().wait(
            project=project,
            region=trim_self_link(operation["region"]),
            operation=operation["name"],
        )
    else:
        req = lookup().compute.globalOperations().wait(
            project=project, operation=operation["name"]
        )
    return req


def wait_for_operation(operation) -> Dict[str, Any]:
    """wait for given operation"""
    project = parse_self_link(operation["selfLink"]).project
    wait_req = wait_request(operation, project=project)

    while True:
        result = ensure_execute(wait_req)
        if result["status"] == "DONE":
            log_errors = " with errors" if "error" in result else ""
            log.debug(
                f"operation complete{log_errors}: type={result['operationType']}, name={result['name']}"
            )
            return result



def getThreadsPerCore(template) -> int:
    if not template.machine_type.supports_smt:
        return 1
    return template.advancedMachineFeatures.threadsPerCore or 2


@retry(
    max_retries=9,
    init_wait_time=1,
    warn_msg="Temporary failure in name resolution",
    exc_type=socket.gaierror,
)
def host_lookup(host_name: str) -> str:
    return socket.gethostbyname(host_name)


class Dumper(yaml.SafeDumper):
    """Add representers for pathlib.Path and NSDict for yaml serialization"""

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.add_representer(NSDict, self.represent_nsdict)
        self.add_multi_representer(Path, self.represent_path)

    @staticmethod
    def represent_nsdict(dumper, data):
        return dumper.represent_mapping("tag:yaml.org,2002:map", data.items())

    @staticmethod
    def represent_path(dumper, path):
        return dumper.represent_scalar("tag:yaml.org,2002:str", str(path))


@dataclass(frozen=True)
class ReservationDetails:
    project: str
    zone: str
    name: str
    policies: List[str] # names (not URLs) of resource policies
    bulk_insert_name: str # name in format suitable for bulk insert (currently identical to user supplied name in long format)
    deployment_type: Optional[str]
    reservation_mode: Optional[str]

    @property
    def dense(self) -> bool:
        return self.deployment_type == "DENSE"

    @property
    def calendar(self) -> bool:
        return self.reservation_mode == "CALENDAR"

@dataclass(frozen=True)
class FutureReservation:
    project: str
    zone: str
    name: str
    specific: bool
    start_time: datetime
    end_time: datetime
    reservation_mode: Optional[str]
    active_reservation: Optional[ReservationDetails]

    @property
    def calendar(self) -> bool:
        return self.reservation_mode == "CALENDAR"

@dataclass
class Job:
    id: int
    name: Optional[str] = None
    required_nodes: Optional[str] = None
    job_state: Optional[str] = None
    duration: Optional[timedelta] = None

@dataclass(frozen=True)
class NodeState:
    base: str
    flags: frozenset

class Lookup:
    """Wrapper class for cached data access"""

    def __init__(self, cfg):
        self._cfg = cfg

    @property
    def cfg(self):
        return self._cfg

    @property
    def project(self):
        return self.cfg.project or authentication_project()

    @cached_property
    def control_addr(self) -> Optional[str]:
        return self.cfg.get("slurm_control_addr", None)

    @property
    def control_host(self):
        return self.cfg.slurm_control_hosts[0]
    
    @cached_property
    def control_hosts(self):
        return self.cfg.slurm_control_hosts

    @cached_property
    def control_host_addr(self):
        return self.control_addr or host_lookup(self.cfg.slurm_control_hosts[0])

    @property
    def control_host_port(self):
        return self.cfg.slurm_control_host_port

    @property
    def endpoint_versions(self):
        return self.cfg.endpoint_versions

    @property
    def scontrol(self):
        return Path(self.cfg.slurm_bin_dir or "") / "scontrol"

    @cached_property
    def instance_role(self):
        return instance_role()

    @cached_property
    def instance_role_safe(self):
        try:
            role = self.instance_role
        except Exception as e:
            log.error(e)
            role = None
        return role

    @property
    def is_controller(self):
        return self.instance_role_safe == "controller"
    @property
    def is_primary_controller(self):
        return self.hostname == self.cfg.slurm_control_hosts[0]
    @property
    def is_login_node(self):
        return self.instance_role_safe == "login"

    @cached_property
    def compute(self):
        # TODO evaluate when we need to use google_app_cred_path
        if self.cfg.google_app_cred_path:
            os.environ["GOOGLE_APPLICATION_CREDENTIALS"] = self.cfg.google_app_cred_path
        return compute_service()

    @cached_property
    def hostname(self):
        if self.instance_role == "controller":
            return instance_metadata("attributes/hostname")
        return socket.gethostname()

    @cached_property
    def hostname_fqdn(self):
        return socket.getfqdn()

    @cached_property
    def zone(self):
        return instance_metadata("zone")

    node_desc_regex = re.compile(
        r"^(?P<prefix>(?P<cluster>[^\s\-]+)-(?P<nodeset>\S+))-(?P<node>(?P<suffix>\w+)|(?P<range>\[[\d,-]+\]))$"
    )

    @lru_cache(maxsize=None)
    def _node_desc(self, node_name):
        """Get parts from node name"""
        if not node_name:
            node_name = self.hostname
        # workaround below is for VMs whose hostname is FQDN
        node_name_short = node_name.split(".")[0]
        m = self.node_desc_regex.match(node_name_short)
        if not m:
            raise Exception(f"node name {node_name} is not valid")
        return m.groupdict()

    def node_prefix(self, node_name=None):
        return self._node_desc(node_name)["prefix"]

    def node_index(self, node: str) -> int:
        """ node_index("cluster-nodeset-45") == 45 """
        suff = self._node_desc(node)["suffix"]

        if suff is None:
            raise ValueError(f"Node {node} name does not end with numeric index")
        return int(suff)

    def node_nodeset_name(self, node_name=None):
        return self._node_desc(node_name)["nodeset"]

    def node_nodeset(self, node_name=None):
        nodeset_name = self.node_nodeset_name(node_name)
        if nodeset_name in self.cfg.nodeset_tpu:
            return self.cfg.nodeset_tpu[nodeset_name]

        return self.cfg.nodeset[nodeset_name]

    def partition_is_tpu(self, part: str) -> bool:
        """check if partition with name part contains a nodeset of type tpu"""
        return len(self.cfg.partitions[part].partition_nodeset_tpu) > 0


    def node_is_tpu(self, node_name=None):
        nodeset_name = self.node_nodeset_name(node_name)
        return self.cfg.nodeset_tpu.get(nodeset_name) is not None

    def nodeset_is_tpu(self, nodeset_name=None) -> bool:
        return self.cfg.nodeset_tpu.get(nodeset_name) is not None

    def node_is_fr(self, node_name:str) -> bool:
        return bool(self.node_nodeset(node_name).future_reservation)

    def is_dormant_fr_node(self, node_name:str) -> bool:
        fr = self.future_reservation(self.node_nodeset(node_name))
        if not fr:
            return False
        return fr.active_reservation is None

    def node_is_dyn(self, node_name=None) -> bool:
        nodeset = self.node_nodeset_name(node_name)
        return self.cfg.nodeset_dyn.get(nodeset) is not None

    def node_template(self, node_name=None) -> str:
        """ Self link of nodeset template """
        return self.node_nodeset(node_name).instance_template

    def node_template_info(self, node_name=None):
        return self.template_info(self.node_template(node_name))

    def node_region(self, node_name=None):
        nodeset = self.node_nodeset(node_name)
        return parse_self_link(nodeset.subnetwork).region

    def nodeset_accelerator_topology(self, nodeset_name: str) -> Optional[str]:
        if not self.nodeset_is_tpu(nodeset_name):
            return self.cfg.nodeset[nodeset_name].get('accelerator_topology')
        return None

    def nodeset_prefix(self, nodeset_name):
        return f"{self.cfg.slurm_cluster_name}-{nodeset_name}"

    def nodelist_range(self, nodeset_name: str, start: int, count: int) -> str:
        assert 0 <= start and 0 < count
        pref = self.nodeset_prefix(nodeset_name)
        if count == 1:
            return f"{pref}-{start}"
        return f"{pref}-[{start}-{start + count - 1}]"

    def static_dynamic_sizes(self, nodeset: NSDict) -> Tuple[int, int]:
        return (nodeset.node_count_static or 0, nodeset.node_count_dynamic_max or 0)

    def nodelist(self, nodeset) -> str:
        cnt = sum(self.static_dynamic_sizes(nodeset))
        if cnt == 0:
            return ""
        return self.nodelist_range(nodeset.nodeset_name, 0, cnt)

    def nodenames(self, nodeset) -> Tuple[Iterable[str], Iterable[str]]:
        pref = self.nodeset_prefix(nodeset.nodeset_name)
        s_count, d_count = self.static_dynamic_sizes(nodeset)
        return (
            (f"{pref}-{i}" for i in range(s_count)),
            (f"{pref}-{i}" for i in range(s_count, s_count + d_count)),
        )

    def power_managed_nodesets(self) -> Iterable[NSDict]:
        return chain(self.cfg.nodeset.values(), self.cfg.nodeset_tpu.values())

    def is_power_managed_node(self, node_name: str) -> bool:
        try:
            ns = self.node_nodeset(node_name)
            if ns is None:
                return False
            idx = int(self._node_desc(node_name)["suffix"])
            return idx < sum(self.static_dynamic_sizes(ns))
        except Exception:
            return False

    def is_static_node(self, node_name: str) -> bool:
        if not self.is_power_managed_node(node_name):
            return False
        idx = int(self._node_desc(node_name)["suffix"])
        return idx < self.node_nodeset(node_name).node_count_static

    @lru_cache(maxsize=None)
    def slurm_nodes(self) -> Dict[str, NodeState]:
        def parse_line(node_line) -> Tuple[str, NodeState]:
            """turn node,state line to (node, NodeState)"""
            # state flags include: CLOUD, COMPLETING, DRAIN, FAIL, POWERED_DOWN,
            #   POWERING_DOWN
            node, fullstate = node_line.split(",")
            state = fullstate.split("+")
            state_tuple = NodeState(base=state[0], flags=frozenset(state[1:]))
            return (node, state_tuple)

        cmd = (
            f"{self.scontrol} show nodes | "
            r"grep -oP '^NodeName=\K(\S+)|\s+State=\K(\S+)' | "
            r"paste -sd',\n'"
        )
        node_lines = run(cmd, shell=True).stdout.rstrip().splitlines()
        nodes = {
            node: state
            for node, state in map(parse_line, node_lines)
            if "CLOUD" in state.flags or "DYNAMIC_NORM" in state.flags
        }
        return nodes

    def node_state(self, nodename: str) -> Optional[NodeState]:
        state = self.slurm_nodes().get(nodename)
        if state is not None:
            return state

        # state is None => Slurm doesn't know this node,
        # there are two reasons:
        # * happy:
        #   * node belongs to removed nodeset
        #   * node belongs to downsized portion of nodeset
        #   * dynamic node that didn't register itself
        # * unhappy:
        #   * there is a drift in Slurm and SlurmGCP configurations
        #   * `slurm_nodes` function failed to handle `scontrol show nodes`,
        #      TODO: make `slurm_nodes` robust by using `scontrol show nodes --json`
        # In either of "unhappy" cases it's too dangerous to proceed - abort slurmsync.
        try:
            ns = self.node_nodeset(nodename)
        except:
            log.info(f"Unknown node {nodename}, belongs to unknown nodeset")
            return None # Can't find nodeset, may be belongs to removed nodeset

        if self.node_is_dyn(nodename):
            log.info(f"Unknown node {nodename}, belongs to dynamic nodeset")
            return None # we can't make any judjment for dynamic nodes

        cnt = sum(self.static_dynamic_sizes(ns))
        if self.node_index(nodename) >= cnt:
            log.info(f"Unknown node {nodename}, out of nodeset size boundaries ({cnt})")
            return None # node belongs to downsized nodeset

        raise RuntimeError(f"Slurm does not recognize node {nodename}, potential misconfiguration.")


    @lru_cache(maxsize=1)
    def instances(self) -> Dict[str, Instance]:
        instance_information_fields = [
            "creationTimestamp",
            "name",
            "resourceStatus",
            "scheduling",
            "status",
            "labels.slurm_instance_role",
            "zone",
            "metadata",
        ]

        instance_fields = ",".join(sorted(instance_information_fields))
        fields = f"items.zones.instances({instance_fields}),nextPageToken"
        flt = f"labels.slurm_cluster_name={self.cfg.slurm_cluster_name} AND name:{self.cfg.slurm_cluster_name}-*"
        act = self.compute.instances()
        op = act.aggregatedList(project=self.project, fields=fields, filter=flt)

        instances = {}
        while op is not None:
            result = ensure_execute(op)
            for zone in result.get("items", {}).values():
                for jo in zone.get("instances", []):
                    inst = Instance.from_json(jo)
                    if inst.name in instances:
                        log.error(f"Duplicate VM name {inst.name} across multiple zones")
                    instances[inst.name] = inst
            op = act.aggregatedList_next(op, result)
        return instances

    def instance(self, instance_name: str) -> Optional[Instance]:
        return self.instances().get(instance_name)

    @lru_cache()
    def _get_reservation(self, project: str, zone: str, name: str) -> Any:
        """See https://cloud.google.com/compute/docs/reference/rest/v1/reservations"""
        return self.compute.reservations().get(
            project=project, zone=zone, reservation=name).execute()

    @lru_cache()
    def get_mig(self, project: str, zone: str, self_link:str) -> Any:
        """https://cloud.google.com/compute/docs/reference/rest/v1/instanceGroupManagers"""
        return self.compute.instanceGroupManagers().get(project=project, zone=zone, instanceGroupManager=self_link).execute()

    @lru_cache
    def get_mig_instances(self, project: str, zone: str, self_link:str) -> Any:
        return self.compute.instanceGroupManagers().listManagedInstances(project=project, zone=zone, instanceGroupManager=self_link).execute() 

    @lru_cache()
    def get_mig_list(self, project: str, zone: str) -> Any:
        """https://cloud.google.com/compute/docs/reference/rest/v1/instanceGroupManagers"""
        return self.compute.instanceGroupManagers().list(project=project, zone=zone).execute()

    @lru_cache()
    def _get_future_reservation(self, project:str, zone:str, name: str) -> Any:
        """See https://cloud.google.com/compute/docs/reference/rest/v1/futureReservations"""
        return self.compute.futureReservations().get(project=project, zone=zone, futureReservation=name).execute()

    def get_reservation_details(self, project:str, zone:str, name:str, bulk_insert_name:str) -> ReservationDetails:
        reservation = self._get_reservation(project, zone, name)

        # Converts policy URLs to names, e.g.:
        # projects/111111/regions/us-central1/resourcePolicies/zebra -> zebra
        policies = [u.split("/")[-1] for u in reservation.get("resourcePolicies", {}).values()]

        return ReservationDetails(
            project=project,
            zone=zone,
            name=name,
            policies=policies,
            deployment_type=reservation.get("deploymentType"),
            reservation_mode=reservation.get("reservationMode"),
            bulk_insert_name=bulk_insert_name)

    def nodeset_reservation(self, nodeset: NSDict) -> Optional[ReservationDetails]:
        if not nodeset.reservation_name:
            return None

        zones = list(nodeset.zone_policy_allow or [])
        assert len(zones) == 1, "Only single zone is supported if using a reservation"
        zone = zones[0]

        regex = re.compile(r'^projects/(?P<project>[^/]+)/reservations/(?P<reservation>[^/]+)(/.*)?$')
        if not (match := regex.match(nodeset.reservation_name)):
            raise ValueError(
                f"Invalid reservation name: '{nodeset.reservation_name}', expected format is 'projects/PROJECT/reservations/NAME'"
            )

        project, name = match.group("project", "reservation")
        return self.get_reservation_details(project, zone, name, nodeset.reservation_name)

    def future_reservation(self, nodeset: NSDict) -> Optional[FutureReservation]:
        if not nodeset.future_reservation:
            return None

        active_reservation = None
        match = re.search(r'^projects/(?P<project>[^/]+)/zones/(?P<zone>[^/]+)/futureReservations/(?P<name>[^/]+)(/.*)?$', nodeset.future_reservation)
        assert match, f"Invalid future reservation name '{nodeset.future_reservation}'"
        project, zone, name = match.group("project","zone","name")
        fr = self._get_future_reservation(project,zone,name)

        start_time = parse_gcp_timestamp(fr["timeWindow"]["startTime"])
        end_time = parse_gcp_timestamp(fr["timeWindow"]["endTime"])

        if "autoCreatedReservations" in fr["status"] and (res:=fr["status"]["autoCreatedReservations"][0]):
            if start_time <= now() <=end_time:
                match = re.search(r'projects/(?P<project>[^/]+)/zones/(?P<zone>[^/]+)/reservations/(?P<name>[^/]+)(/.*)?$',res)
                assert match, f"Unexpected reservation name '{res}'"
                res_name = match.group("name")
                bulk_insert_name = f"projects/{project}/reservations/{res_name}"
                active_reservation = self.get_reservation_details(project, zone, res_name, bulk_insert_name)

        return FutureReservation(
            project=project,
            zone=zone,
            name=name,
            specific=fr["specificReservationRequired"],
            start_time=start_time,
            end_time=end_time,
            reservation_mode=fr.get("reservationMode"),
            active_reservation=active_reservation
        )

    @lru_cache(maxsize=1)
    def machine_types(self):
        field_names = "name,zone,guestCpus,memoryMb,accelerators"
        fields = f"items.zones.machineTypes({field_names}),nextPageToken"

        machines: Dict[str, Dict[str, Any]] = defaultdict(dict)
        act = self.compute.machineTypes()
        op = act.aggregatedList(project=self.project, fields=fields)
        while op is not None:
            result = ensure_execute(op)
            machine_iter = chain.from_iterable(
                scope.get("machineTypes", []) for scope in result["items"].values()
            )
            for machine in machine_iter:
                name = machine["name"]
                zone = machine["zone"]
                machines[name][zone] = machine

            op = act.aggregatedList_next(op, result)
        return machines

    def machine_type(self, name: str) -> MachineType:
        custom_patt = re.compile(
            r"((?P<family>\w+)-)?custom-(?P<cpus>\d+)-(?P<mem>\d+)"
        )
        if match := custom_patt.match(name):
            return MachineType(
                name=name,
                guest_cpus=int(match.group("cpus")),
                memory_mb=int(match.group("mem")),
                accelerators=[],
            )

        machines = self.machine_types()
        if name not in machines:
            raise Exception(f"machine type {name} not found")
        per_zone = machines[name]
        assert per_zone
        return MachineType.from_json(
            next(iter(per_zone.values())) # pick the first/any zone
        )

    def template_machine_conf(self, template_link):
        template = self.template_info(template_link)
        machine = template.machine_type

        machine_conf = NSDict()
        machine_conf.boards = 1  # No information, assume 1
        machine_conf.sockets = machine.sockets
        # the value below for SocketsPerBoard must be type int
        machine_conf.sockets_per_board = machine_conf.sockets // machine_conf.boards
        machine_conf.threads_per_core = 1
        _div = 2 if getThreadsPerCore(template) == 1 else 1
        machine_conf.cpus = (
            int(machine.guest_cpus / _div) if machine.supports_smt else machine.guest_cpus
        )
        machine_conf.cores_per_socket = int(machine_conf.cpus / machine_conf.sockets)
        # Because the actual memory on the host will be different than
        # what is configured (e.g. kernel will take it). From
        # experiments, about 16 MB per GB are used (plus about 400 MB
        # buffer for the first couple of GB's. Using 30 MB to be safe.
        gb = machine.memory_mb // 1024
        machine_conf.memory = machine.memory_mb - (400 + (30 * gb))
        return machine_conf

    @lru_cache(maxsize=None)
    def template_info(self, template_link):
        template_name = trim_self_link(template_link)
        cache = file_cache.cache("template_cache")

        if cached := cache.get(template_name):
            return NSDict(cached)

        template = ensure_execute(
            self.compute.instanceTemplates().get(
                project=self.project, instanceTemplate=template_name
            )
        ).get("properties")
        template = NSDict(template)
        # name and link are not in properties, so stick them in
        template.name = template_name
        template.link = template_link
        template.machine_type = self.machine_type(template.machineType)
        # TODO delete metadata to reduce memory footprint?
        # del template.metadata

        # translate gpus into an easier-to-read format
        if template.machine_type.accelerators:
            template.gpu = template.machine_type.accelerators[0]
        elif template.guestAccelerators:
            tga = template.guestAccelerators[0]
            template.gpu = AcceleratorInfo(
                type=tga.acceleratorType,
                count=tga.acceleratorCount)
        else:
            template.gpu = None

        cache.set(template_name, template.to_dict())
        return template


    def _parse_job_info(self, job_info: str) -> Job:
        """Extract job details"""
        if match:= re.search(r"JobId=(\d+)", job_info):
            job_id = int(match.group(1))
        else:
            raise ValueError(f"Job ID not found in the job info: {job_info}")

        if match:= re.search(r"TimeLimit=(?:(\d+)-)?(\d{2}):(\d{2}):(\d{2})", job_info):
          days, hours, minutes, seconds = match.groups()
          duration = timedelta(
              days=int(days) if days else 0,
              hours=int(hours),
              minutes=int(minutes),
              seconds=int(seconds)
          )
        else:
            duration = None

        if match := re.search(r"JobName=([^\n]+)", job_info):
            name = match.group(1)
        else:
            name = None

        if match := re.search(r"JobState=(\w+)", job_info):
            job_state = match.group(1)
        else:
            job_state = None

        if match := re.search(r"ReqNodeList=([^ ]+)", job_info):
            required_nodes = match.group(1)
        else:
            required_nodes = None

        return Job(id=job_id, duration=duration, name=name, job_state=job_state, required_nodes=required_nodes)

    @lru_cache
    def get_jobs(self) -> List[Job]:
        res = run(f"{self.scontrol} show jobs", timeout=30)

        return [self._parse_job_info(job) for job in res.stdout.split("\n\n")[:-1]]

    @lru_cache
    def job(self, job_id: int) -> Optional[Job]:
        job_info = run(f"{self.scontrol} show jobid {job_id}", check=False).stdout.rstrip()
        if not job_info:
            return None

        return self._parse_job_info(job_info=job_info)

    @property
    def etc_dir(self) -> Path:
        return Path(self.cfg.output_dir or slurmdirs.etc)

    def controller_mount_server_ip(self) -> str:
        return self.control_addr or self.control_host

    def normalize_ns_mount(self, ns: Union[dict, NSMount]) -> NSMount:
        if isinstance(ns, NSMount):
            return ns

        server_ip = ns.get("server_ip") or "$controller"
        if server_ip == "$controller":
            server_ip = self.controller_mount_server_ip()

        return NSMount(
            server_ip=server_ip,
            local_mount=Path(ns["local_mount"]),
            remote_mount=Path(ns["remote_mount"]),
            fs_type=ns["fs_type"],
            mount_options=ns["mount_options"],
        )

    @property
    def munge_mount(self) -> NSMount:
        if self.cfg.munge_mount:
            mnt = self.cfg.munge_mount
            mnt.local_mount = mnt.local_mount or "/mnt/munge"
            return self.normalize_ns_mount(mnt)
        else:
            return NSMount(
                server_ip=self.controller_mount_server_ip(),
                local_mount=Path("/mnt/munge"),
                remote_mount=dirs.munge,
                fs_type="nfs",
                mount_options="defaults,hard,intr,_netdev",
            )

    @property
    def slurm_key_mount(self) -> NSMount:
        if self.cfg.slurm_key_mount:
            mnt = self.cfg.slurm_key_mount
            mnt.local_mount = mnt.local_mount or slurmdirs.key_distribution
            return self.normalize_ns_mount(mnt)
        else:
            return NSMount(
                server_ip=self.controller_mount_server_ip(),
                local_mount=slurmdirs.key_distribution,
                remote_mount=slurmdirs.key_distribution,
                fs_type="nfs",
                mount_options="defaults,hard,intr,_netdev",
            )

    def is_flex_node(self, node: str) -> bool:
        try:
            nodeset = self.node_nodeset(node)
            if nodeset.dws_flex.use_bulk_insert:
                return False #For legacy flex support
            return bool(nodeset.dws_flex.enabled)
        except:
            return False

    def is_provisioning_flex_node(self, node:str) -> bool:
        if not self.is_flex_node(node):
            return False
        if self.instance(node) is not None:
            return True

        nodeset = self.node_nodeset(node)
        zones = nodeset.zone_policy_allow
        assert len(zones) == 1
        zone = zones[0]

        potential_migs=[]
        mig_list=self.get_mig_list(self.project, zone)
        
        if not mig_list or not mig_list.get("items"):
            return False

        for mig in mig_list["items"]:
            if not mig.get("instanceTemplate"): #possibly an old MIG
                return False
            if mig["instanceTemplate"] == self.node_template(node) and mig["currentActions"]["creating"] > 0:
                potential_migs.append(self.get_mig_instances(self.project, zone, trim_self_link(mig["selfLink"])))

        if not potential_migs:
            return False

        for instance_collection in potential_migs[0]["managedInstances"]:
            if node in instance_collection["name"] and instance_collection["currentAction"]=="CREATING":
                return True
        return False
    
    def cluster_regions(self) -> list[str]:
        """
        Returns all regions used in cluster
        NOTE: only concerned with normal nodesets, 
        neither TPU, nor dynamic, nor login node, nor controller node are considered
        """
        res = set()
        for nodeset in self.cfg.nodeset.values():
            res.add(parse_self_link(nodeset.subnetwork).region)
        return list(res)



_lkp: Optional[Lookup] = None

def _load_config() -> NSDict:
    return NSDict(yaml.safe_load(CONFIG_FILE.read_text()))

def lookup() -> Lookup:
    global _lkp
    if _lkp is None:
        try:
            cfg =  _load_config()
        except FileNotFoundError:
            log.error(f"config file not found: {CONFIG_FILE}")
            cfg = NSDict() # TODO: fail here, once all code paths are covered (mainly init_logging)
        _lkp = Lookup(cfg)
    return _lkp

def update_config(cfg: NSDict) -> None:
    global _lkp
    _lkp = Lookup(cfg)

def scontrol_reconfigure(lkp: Lookup) -> None:
    log.info("Running scontrol reconfigure")
    run(f"{lkp.scontrol} reconfigure")
