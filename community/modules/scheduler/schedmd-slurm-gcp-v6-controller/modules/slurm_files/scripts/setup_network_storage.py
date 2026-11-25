#!/slurm/python/venv/bin/python3.13

# Copyright (C) SchedMD LLC.
# Copyright 2026 Google LLC
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

from typing import Iterable, List, Optional, Set, Union

import ipaddress
import os
import posixpath
import random
import re
import socket
import sys
import stat
import time
import logging
import uuid

import shutil
from pathlib import Path
from concurrent.futures import as_completed
from util import NSDict

import util
from util import NSMount, lookup, run, dirs, separate
from more_executors import Executors, ExceptionRetryPolicy


log = logging.getLogger()

def mounts_by_local(mounts: list[NSMount]) -> dict[str, NSMount]:
    """convert list of mounts to dict of mounts, local_mount as key"""
    return {str(m.local_mount.resolve()): m for m in mounts}


def _get_default_mounts(lkp: util.Lookup) -> list[NSMount]:
    if lkp.cfg.disable_default_mounts:
        return []
    return [
        NSMount(
                server_ip=lkp.controller_mount_server_ip(),
                remote_mount=path,
                local_mount=path,
                fs_type="nfs",
                mount_options="defaults,hard,intr",
        )
        for path in (
            dirs.home,
            dirs.apps,
        )
    ]

def get_slurm_bucket_mount() -> NSMount:
    bucket, path = util._get_bucket_and_common_prefix()
    return  NSMount(
        fs_type="gcsfuse",
        server_ip="",
        remote_mount=Path(bucket),
        local_mount=dirs.slurm_bucket_mount,
        mount_options=f"defaults,_netdev,implicit_dirs,only_dir={path}",
    )

def resolve_network_storage() -> List[NSMount]:
    """Combine appropriate network_storage fields to a single list"""
    lkp = lookup()

    # create dict of mounts, local_mount: mount_info
    mounts = mounts_by_local(_get_default_mounts(lkp))

    if lkp.is_controller and util.should_mount_slurm_bucket():
        mounts.update(mounts_by_local([get_slurm_bucket_mount()]))

    # On non-controller instances, entries in network_storage could overwrite
    # default exports from the controller. Be careful, of course
    common = [lkp.normalize_ns_mount(m) for m in lkp.cfg.network_storage]
    mounts.update(mounts_by_local(common))

    if lkp.is_login_node:
        login_group = lkp.cfg.login_groups[util.instance_login_group()] 
        login_ns = [lkp.normalize_ns_mount(m) for m in login_group.network_storage]
        mounts.update(mounts_by_local(login_ns))

    if lkp.instance_role == "compute":
        try:
            nodeset = lkp.node_nodeset()
        except Exception:
            pass # external nodename, skip lookup
        else:
            nodeset_ns = [lkp.normalize_ns_mount(m) for m in nodeset.network_storage]
            mounts.update(mounts_by_local(nodeset_ns))

    return list(mounts.values())

def is_controller_mount(mount) -> bool:
    if not mount:
        return False
    if getattr(mount, "fs_type", None) == "gcsfuse":
        return False
    if not getattr(mount, "server_ip", None):
        return lookup().is_controller
    # NOTE: Valid Lustre server_ip can take the form of '<IP>@tcp'
    server_ip = str(mount.server_ip).split("@")[0]
    try:
        mount_addr = util.host_lookup(server_ip) if server_ip else None
    except Exception:
        mount_addr = None
    control_host = lookup().control_host
    control_host_addr = lookup().control_host_addr
    return (
        (mount_addr is not None and mount_addr == control_host_addr)
        or (control_host is not None and server_ip == control_host)
        or (lookup().is_controller and (
            server_ip in ("127.0.0.1", "localhost")
            or server_ip == lookup().hostname
            or mount_addr in ("127.0.0.1", "localhost")
        ))
    )


def _is_controller_nfs_server(server: str, lkp: util.Lookup) -> bool:
    """Check if an NFS server string refers to the controller without retrying DNS lookups."""
    if not server:
        return False
    srv_fn = getattr(lkp, "controller_mount_server_ip", None)
    host = getattr(lkp, "control_host", None)
    addr = getattr(lkp, "control_addr", None)
    for cand in (srv_fn() if callable(srv_fn) else None, host, addr):
        if isinstance(cand, str) and cand and server == cand:
            return True
    try:
        server_ip = socket.gethostbyname(server)
    except OSError:
        return False
    if isinstance(addr, str) and addr and server_ip == addr:
        return True
    try:
        return bool(isinstance(host, str) and host and server_ip == socket.gethostbyname(host))
    except OSError:
        return False


def _probe_tcp_port(host: str, port: int = 2049, timeout: float = 1.0) -> bool:
    """Check if TCP port is accepting connections (pure Python, IPv4/IPv6 compatible)."""
    if not host or not host.strip():
        return False
    try:
        with socket.create_connection((host.strip(), port), timeout=timeout):
            return True
    except (OSError, TypeError, OverflowError):
        return False


def _is_path_exported(expected_path: str, active_exports: Set[str]) -> bool:
    """Check if expected_path matches an active export or is a subdirectory of one."""
    if not expected_path or not expected_path.startswith("/"):
        return False
    norm_p = posixpath.normpath("/" + expected_path.lstrip("/"))
    for exp in active_exports:
        if exp and exp.startswith("/"):
            norm_e = posixpath.normpath("/" + exp.lstrip("/"))
            if norm_p == norm_e or norm_p.startswith(norm_e.rstrip("/") + "/"):
                return True
    return False


def _check_nfs_exports_showmount(
    server: str, expected_paths: Set[str], timeout: float = 3.0
) -> Optional[bool]:
    """Verify exports via showmount -e if available and port 111 is reachable."""
    path_env = f"/usr/sbin:/sbin:{os.environ.get('PATH', '')}"
    showmount_bin = shutil.which("showmount", path=path_env)
    if not showmount_bin or not _probe_tcp_port(server, port=111, timeout=0.5):
        return None
    env = {**os.environ, "LC_ALL": "C", "PATH": path_env}
    try:
        res = run([showmount_bin, "--no-headers", "-e", server], timeout=timeout, check=False, env=env)
        if res.returncode != 0:
            return None
        active_exports = {
            posixpath.normpath("/" + line.split()[0].lstrip("/"))
            for line in res.stdout.splitlines()
            if line.strip() and not line.strip().startswith("Export list") and line.split()[0].startswith("/")
        }
        missing = {p for p in expected_paths if not _is_path_exported(p, active_exports)}
        if not missing:
            log.info(f"showmount confirmed exports {sorted(expected_paths)} on {server}.")
            return True
        log.debug(f"showmount exports on {server} missing expected shares: {sorted(missing)}")
        return False
    except Exception as e:
        log.debug(f"showmount query on {server} failed with exception: {e}")
        return None


def wait_for_controller_nfs(
    server: str, expected_paths: Iterable[Union[str, Path]], timeout: int = 600
) -> bool:
    """Wait for controller NFS server to accept TCP 2049 and export expected paths."""
    if timeout <= 0:
        log.warning(f"Invalid timeout {timeout}s waiting for controller NFS server '{server}'; skipping wait.")
        return False

    normalized = {posixpath.normpath("/" + str(p).lstrip("/")) for p in expected_paths if str(p)}
    log.info(f"Waiting up to {timeout}s for controller NFS '{server}' to export {sorted(normalized)}...")
    deadline = time.monotonic() + timeout
    port_open = False

    for wait in util.backoff_delay(min(1.5, float(timeout)), timeout=timeout):
        if not port_open:
            port_open = _probe_tcp_port(server, port=2049, timeout=1.0)
            if port_open:
                log.info(f"NFS TCP port 2049 is open on {server}.")
        if port_open:
            if not normalized or _check_nfs_exports_showmount(server, normalized, timeout=3.0) in (True, None):
                log.info(f"Controller NFS exports confirmed ready on {server}.")
                time.sleep(0.5)
                return True
        if time.monotonic() >= deadline:
            break
        time.sleep(min(wait * random.uniform(0.8, 1.2), max(0.0, deadline - time.monotonic())))

    reason = f"NFS port 2049 on {server}" if not port_open else f"controller '{server}' to export {sorted(normalized)}"
    log.warning(f"Timed out after {timeout}s waiting for {reason}. Proceeding to standard mount retries.")
    return False


def setup_network_storage():
    """prepare network fs mounts and add them to fstab"""
    log.info("Set up network storage")
    
    all_mounts = resolve_network_storage()
    if lookup().is_controller:
        mounts, _ = separate(is_controller_mount, all_mounts)
    else:
        mounts = all_mounts

    if not lookup().is_controller:
        lkp = lookup()
        key_mnt = lkp.slurm_key_mount if lkp.cfg.enable_slurm_auth else lkp.munge_mount
        candidates = [*mounts, *([key_mnt] if key_mnt else [])]
        controller_mounts: dict[str, set[str]] = {}
        for m in candidates:
            if (getattr(m, "fs_type", None) or "").lower() == "nfs" and m.server_ip:
                server = str(m.server_ip).split("@")[0]
                if _is_controller_nfs_server(server, lkp):
                    controller_mounts.setdefault(server, set()).add(str(m.remote_mount))

        raw_t = (
            lkp.cfg.get("controller_startup_scripts_timeout")
            if isinstance(lkp.cfg, dict)
            else getattr(lkp.cfg, "controller_startup_scripts_timeout", None)
        )
        startup_t = int(raw_t) if isinstance(raw_t, (int, float)) and not isinstance(raw_t, bool) and raw_t > 0 else 300
        for server, paths in controller_mounts.items():
            wait_for_controller_nfs(server, paths, timeout=max(600, startup_t + 300))

    # Determine fstab entries and write them out
    fstab_entries = []
    for mount in mounts:
        local_mount = mount.local_mount
        fs_type = mount.fs_type
        server_ip = mount.server_ip or ""
        src = mount.remote_mount if fs_type == "gcsfuse" else f"{server_ip}:{mount.remote_mount}"
        
        log.info(f"Setting up mount ({fs_type}) {src} to {local_mount}")
        util.mkdirp(local_mount)

        mount_options = mount.mount_options.split(",") if mount.mount_options else []
        if "_netdev" not in mount_options:
            mount_options += ["_netdev"]
        options_line = ",".join(mount_options)
        
        
        fstab_entries.append(f"{src}   {local_mount}     {fs_type}     {options_line}     0 0")

    fstab = Path("/etc/fstab")
    if not Path(fstab.with_suffix(".bak")).is_file():
        shutil.copy2(fstab, fstab.with_suffix(".bak"))
    shutil.copy2(fstab.with_suffix(".bak"), fstab)
    with open(fstab, "a") as f:
        f.write("\n")
        for entry in fstab_entries:
            f.write(entry)
            f.write("\n")

    mount_fstab(mounts, log)
    if lookup().cfg.enable_slurm_auth:
      slurm_key_mount_handler()
    else:
      munge_mount_handler()


def mount_fstab(mounts: list[NSMount], log):
    """Wait on each mount, then make sure all fstab is mounted"""
    def mount_path(path: Path, mount_owner: Optional[str], mount_perm: Optional[str]):
        log.info(f"Waiting for '{path}' to be mounted...")
        try:
            run(f"mount {path}", timeout=120)
        except Exception as e:
            exc_type, _, _ = sys.exc_info()
            log.error(f"mount of path '{path}' failed: {exc_type}: {e}")
            raise e
        log.info(f"Mount point '{path}' was mounted.")
        if mount_owner and ":" in mount_owner:
            user, group = mount_owner.split(":", 1)
            uid: Union[int, str]
            try:
                uid = int(user)
            except ValueError:
                uid = user
            gid: Union[int, str]
            try:
                gid = int(group)
            except ValueError:
                gid = group
            try:
                shutil.chown(path, user=uid, group=gid)
                log.info(f"Mount point '{path}' changed owner to {user}:{group}.")
            except LookupError as e:
                log.error(f"Failed to resolve owner {user}:{group}: {e}")
                raise e

        if mount_perm:
            mount_perm_int = int(mount_perm, 8)
            os.chmod(path, mount_perm_int)
            log.info(f"Mount point '{path}' changed mode to {mount_perm}.")

    MAX_MOUNT_TIMEOUT = 60 * 5
    future_list = []
    retry_policy = ExceptionRetryPolicy(
        max_attempts=120, exponent=1.6, sleep=1.0, max_sleep=16.0
    )
    with Executors.thread_pool().with_timeout(MAX_MOUNT_TIMEOUT).with_retry(
        retry_policy=retry_policy
    ) as exe:
        for m in mounts:
            future = exe.submit(mount_path, m.local_mount, m.local_mount_owner, m.local_mount_permissions)
            future_list.append(future)

        # Iterate over futures, checking for exceptions
        for future in as_completed(future_list):
            try:
                future.result()
            except Exception as e:
                raise e


def wait_for_file(file_path: Path, timeout: int = 300):
    """Wait for a file to exist and be non-empty at the given path."""
    log.info(f"Waiting up to {timeout}s for file to exist and be ready: {file_path}")

    for retry, wait in enumerate(util.backoff_delay(1.0, timeout), 1):
        try:
            if file_path.exists() and file_path.stat().st_size > 0:
                with open(file_path, 'rb') as f:
                    f.read(1)
                log.info(f"File found and verified: {file_path}")
                return
        except OSError as e:
            log.warning(f"File {file_path} exists but is not ready (try {retry}): {e}")

        if wait > 0:
            time.sleep(wait)

    raise TimeoutError(f"Timeout waiting for file to be ready: {file_path}")


def munge_mount_handler():
    if lookup().is_controller:
        return

    munge_key = Path(dirs.munge / "munge.key")
    # Only in hybrid, elsewhere the controller can regenerate the key and a
    # rebooting node has to pick the new one up.
    if lookup().cfg.hybrid and munge_key.is_file():
        log.info("Key already in place")
        return

    mnt = lookup().munge_mount

    log.info(f"Mounting munge share to: {mnt.local_mount}")
    mnt.local_mount.mkdir()
    if mnt.fs_type == "gcsfuse":
        cmd = [
            "gcsfuse",
            f"--only-dir={mnt.remote_mount}" if mnt.remote_mount != "" else None,
            mnt.server_ip,
            str(mnt.local_mount),
        ]
    else:
        cmd = [
            "mount",
            f"--types={mnt.fs_type}",
            f"--options={mnt.mount_options}" if mnt.mount_options != "" else None,
            f"{mnt.server_ip}:{mnt.remote_mount}",
            str(mnt.local_mount),
        ]
    # wait max 240s for munge mount
    timeout = 240
    for retry, wait in enumerate(util.backoff_delay(0.5, timeout), 1):
        try:
            run(cmd, timeout=timeout)
            break
        except Exception as e:
            log.error(
                f"munge mount failed: '{cmd}' {e}, try {retry}, waiting {wait:0.2f}s"
            )
            time.sleep(wait)
            err = e
            continue
    else:
        raise err

    src_key = Path(mnt.local_mount / "munge.key")
    wait_for_file(src_key)
    log.info(f"Copy munge.key from: {mnt.local_mount}")
    shutil.copy2(src_key, munge_key)

    log.info("Restrict permissions of munge.key")
    shutil.chown(munge_key, user="munge", group="munge")
    os.chmod(munge_key, stat.S_IRUSR)

    log.info(f"Unmount {mnt.local_mount}")
    if mnt.fs_type == "gcsfuse":
        run(f"fusermount -u {mnt.local_mount}", timeout=120)
    else:
        run(f"umount {mnt.local_mount}", timeout=120)
    shutil.rmtree(mnt.local_mount)

def slurm_key_mount_handler():
    if lookup().is_controller:
        return

    file_name = "slurm.key"
    slurm_key = Path(util.slurmdirs.etc / file_name)
    # Only in hybrid, see the comment in munge_mount_handler.
    if lookup().cfg.hybrid and slurm_key.is_file():
        log.info("Key already in place")
        return

    mnt = lookup().slurm_key_mount

    log.info(f"Mounting slurm_key share to: {mnt.local_mount}")
    util.mkdirp(mnt.local_mount)
    if mnt.fs_type == "gcsfuse":
        cmd = [
            "gcsfuse",
            f"--only-dir={mnt.remote_mount}" if mnt.remote_mount != "" else None,
            mnt.server_ip,
            str(mnt.local_mount),
        ]
    else:
        cmd = [
            "mount",
            f"--types={mnt.fs_type}",
            f"--options={mnt.mount_options}" if mnt.mount_options != "" else None,
            f"{mnt.server_ip}:{mnt.remote_mount}",
            str(mnt.local_mount),
        ]
    timeout = 300 # wait max 300s to mount
    for retry, wait in enumerate(util.backoff_delay(0.5, timeout), 1):
        try:
            run(cmd, timeout=timeout)
            break
        except Exception as e:
            log.error(
                f"slurm key mount failed: '{cmd}' {e}, try {retry}, waiting {wait:0.2f}s"
            )
            time.sleep(wait)
            err = e
            continue
    else:
        raise err

    src_key = mnt.local_mount / file_name
    wait_for_file(src_key)
    log.info(f"Copy slurm.key from: {mnt.local_mount}")
    shutil.copy2(src_key, slurm_key)

    log.info("Restrict permissions of slurm.key")
    util.chown_slurm(slurm_key, mode=0o400)

    log.info(f"Unmount {mnt.local_mount}")
    if mnt.fs_type == "gcsfuse":
        run(f"fusermount -u {mnt.local_mount}", timeout=120)
    else:
        run(f"umount {mnt.local_mount}", timeout=120)
    shutil.rmtree(mnt.local_mount)


def _controller_subnet_cidr() -> Optional[str]:
    """Return the CIDR of the controller's primary subnetwork, from instance metadata."""
    try:
        ip = util.instance_metadata("network-interfaces/0/ip", silent=True)
        mask = util.instance_metadata("network-interfaces/0/subnetmask", silent=True)
        if ip and mask:
            return str(ipaddress.ip_network(f"{ip}/{mask}", strict=False))
    except Exception as e:
        log.warning(f"Failed to derive controller subnetwork from metadata: {e}")
    return None


# Broadest client ranges accepted for controller NFS exports. GCP subnetworks
# are at most /8 (IPv4), so anything broader (e.g. 0.0.0.0/0, or 0.0.0.0/1 plus
# 128.0.0.0/1) is a misconfiguration that would export to (nearly) every host.
_MIN_CLIENT_PREFIXLEN = {4: 8, 6: 32}


def nfs_export_clients() -> List[str]:
    """Return the NFS client specs (CIDRs) that controller exports are limited to.

    Never returns the wildcard '*' or ranges broader than /8 (IPv4) or /32
    (IPv6). Uses `nfs_export_clients` from the cluster
    config, which Terraform fills with the CIDR ranges of the controller, login
    and nodeset subnetworks plus any operator-supplied extra ranges. If the
    config has no ranges, falls back to the controller's own subnetwork.
    """
    lkp = util.lookup()
    configured = lkp.cfg.nfs_export_clients
    clients: List[str] = []
    if isinstance(configured, (list, tuple)):
        for c in configured:
            c = str(c).strip() if c else ""
            if not c or c == "*":
                continue
            try:
                net = ipaddress.ip_network(c, strict=False)
            except ValueError:
                log.warning(f"Ignoring invalid NFS export client range '{c}'")
                continue
            if net.prefixlen < _MIN_CLIENT_PREFIXLEN[net.version]:
                log.warning(
                    f"Ignoring NFS export client range '{c}': broader than "
                    f"/{_MIN_CLIENT_PREFIXLEN[net.version]}"
                )
                continue
            c = str(net)
            if c not in clients:
                clients.append(c)
    if clients:
        return clients

    fallback = _controller_subnet_cidr()
    if fallback:
        log.warning(
            "No NFS export client ranges configured; limiting controller NFS exports "
            f"to the controller subnetwork {fallback}"
        )
        return [fallback]
    raise RuntimeError(
        "Unable to determine NFS export client ranges for the controller. "
        "Refusing to export controller NFS shares to all hosts."
    )


def _export_line(path, clients: List[str], options: str) -> str:
    return f"{path}  " + " ".join(f"{c}({options})" for c in clients)


SLURM_EXPORTS_FILE = Path("/etc/exports.d/slurm.exports")


def _existing_fsid(path) -> Optional[str]:
    """Return the fsid of `path` in the current slurm exports file, if any."""
    try:
        text = SLURM_EXPORTS_FILE.read_text()
    except OSError:
        return None
    for line in text.splitlines():
        parts = line.split()
        if not parts or parts[0] != str(path):
            continue
        m = re.search(r"fsid=([^,)\s]+)", line)
        if m:
            return m.group(1)
    return None


def setup_nfs_exports(prepare_paths: bool = True):
    """nfs export all needed directories

    `prepare_paths` creates the exported directories, sets them to 0755 and
    removes matching lines from /etc/exports. It runs at first boot. A manual
    refresh (--refresh-exports) skips it so it does not undo permissions an
    admin has changed since, and only re-exports if the exports file changes.

    Exports are limited to the cluster's subnetwork ranges (see
    nfs_export_clients) instead of any host ('*'):
    - cluster-internal mounts (/home, /opt/apps, ...) are read-write. They use
      no_root_squash unless `nfs_export_root_squash` is set, because login and
      compute nodes write to them as root (for example to create home
      directories on first login).
    - the Slurm/munge key mount is read-only. It needs no_root_squash because
      nodes copy the root-owned 0400 key as root.
    - the Slurm bucket mount is read-only.
    """
    lkp = util.lookup()
    assert lkp.is_controller

    clients = nfs_export_clients()
    log.info(f"Limiting controller NFS exports to: {', '.join(clients)}")

    # The controller only needs to set up exports for cluster-internal mounts
    exported_mounts = [m for m in resolve_network_storage() if is_controller_mount(m)]

    squash = "root_squash" if lkp.cfg.nfs_export_root_squash is True else "no_root_squash"
    # key by remote mount path since that is what needs exporting
    to_export = {m.remote_mount: f"rw,no_subtree_check,{squash}" for m in exported_mounts}

    key_mount = lkp.slurm_key_mount if lkp.cfg.enable_slurm_auth else lkp.munge_mount
    if is_controller_mount(key_mount):
        # Export key mount as read-only
        to_export[key_mount.remote_mount] = "ro,no_subtree_check,no_root_squash"
    
    if util.should_mount_slurm_bucket():
        mnt = get_slurm_bucket_mount()
        # FSID is required for virtual filesystem that is not based on a device
        # Also export it as read-only. Reuse an existing FSID so re-exporting
        # (e.g. on reconfigure) does not leave clients with stale file handles.
        fsid = _existing_fsid(mnt.local_mount) or str(uuid.uuid4())
        to_export[mnt.local_mount] = f"ro,no_subtree_check,no_root_squash,fsid={fsid}"

    # export path if corresponding selector boolean is True
    lines = []
    for path, options in to_export.items():
        if prepare_paths:
            util.mkdirp(Path(path))
            try:
                os.chmod(Path(path), 0o755)
            except OSError as e:
                log.warning(f"Failed to set permissions for {path}: {e}")
            run(rf"sed -i '\#{path}#d' /etc/exports", timeout=30)
        lines.append(_export_line(path, clients, options))

    content = "\n" + "\n".join(lines)
    if not prepare_paths:
        try:
            if SLURM_EXPORTS_FILE.read_text() == content:
                log.info("Controller NFS exports are unchanged")
                return
        except OSError:
            pass
    util.mkdirp(SLURM_EXPORTS_FILE.parent)
    with SLURM_EXPORTS_FILE.open("w") as f:
        f.write(content)
    run("exportfs -ra", timeout=30)


if __name__ == "__main__":
    # Manual refresh, run as root on the controller after changes to the
    # cluster's subnetworks or extra_nfs_client_cidrs, or once after upgrading
    # an existing controller (first-boot setup does not run again).
    import argparse

    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--refresh-exports",
        action="store_true",
        help="Recompute and apply controller NFS exports from the cluster config",
    )
    args = util.init_log_and_parse(parser)
    if args.refresh_exports:
        if os.geteuid() != 0:
            sys.exit("--refresh-exports must be run as root")
        # Read the latest config from the bucket so the refresh does not depend
        # on slurmsync having fetched it yet. Do not write the local config or
        # its hash, so slurmsync still detects and applies the change itself.
        fetched = util._fetch_config(old_hash=None)
        if fetched:
            util.update_config(fetched[0])
        setup_nfs_exports(prepare_paths=False)
