#!/slurm/python/venv/bin/python3.13

# Copyright (C) SchedMD LLC.
# Copyright 2024 Google LLC
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

from typing import List

import os
import sys
import stat
import time
import logging
import uuid

import shutil
from pathlib import Path
from concurrent.futures import as_completed
from addict import Dict as NSDict # type: ignore

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
    # NOTE: Valid Lustre server_ip can take the form of '<IP>@tcp'
    server_ip = mount.server_ip.split("@")[0]
    mount_addr = util.host_lookup(server_ip)
    return mount_addr == lookup().control_host_addr

def setup_network_storage():
    """prepare network fs mounts and add them to fstab"""
    log.info("Set up network storage")
    
    all_mounts = resolve_network_storage()
    if lookup().is_controller:
        mounts, _ = separate(is_controller_mount, all_mounts)
    else:
        mounts = all_mounts

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
    def mount_path(path: Path):
        log.info(f"Waiting for '{path}' to be mounted...")
        try:
            run(f"mount {path}", timeout=120)
        except Exception as e:
            exc_type, _, _ = sys.exc_info()
            log.error(f"mount of path '{path}' failed: {exc_type}: {e}")
            raise e
        log.info(f"Mount point '{path}' was mounted.")

    MAX_MOUNT_TIMEOUT = 60 * 5
    future_list = []
    retry_policy = ExceptionRetryPolicy(
        max_attempts=120, exponent=1.6, sleep=1.0, max_sleep=16.0
    )
    with Executors.thread_pool().with_timeout(MAX_MOUNT_TIMEOUT).with_retry(
        retry_policy=retry_policy
    ) as exe:
        for m in mounts:
            future = exe.submit(mount_path, m.local_mount)
            future_list.append(future)

        # Iterate over futures, checking for exceptions
        for future in as_completed(future_list):
            try:
                future.result()
            except Exception as e:
                raise e


def munge_mount_handler():
    if lookup().is_controller:
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

    munge_key = Path(dirs.munge / "munge.key")
    log.info(f"Copy munge.key from: {mnt.local_mount}")
    shutil.copy2(Path(mnt.local_mount / "munge.key"), munge_key)

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
    mnt = lookup().slurm_key_mount

    log.info(f"Mounting slurm_key share to: {mnt.local_mount}")
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
    timeout = 120 # wait max 120s to mount
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

    file_name = "slurm.key"
    dst = Path(util.slurmdirs.etc / file_name)
    log.info(f"Copy slurm.key from: {mnt.local_mount}")
    shutil.copy2(mnt.local_mount / file_name, dst)

    log.info("Restrict permissions of slurm.key")
    util.chown_slurm(dst, mode=0o400)

    log.info(f"Unmount {mnt.local_mount}")
    if mnt.fs_type == "gcsfuse":
        run(f"fusermount -u {mnt.local_mount}", timeout=120)
    else:
        run(f"umount {mnt.local_mount}", timeout=120)
    shutil.rmtree(mnt.local_mount)


def setup_nfs_exports():
    """nfs export all needed directories"""
    lkp = util.lookup()
    assert lkp.is_controller

    # The controller only needs to set up exports for cluster-internal mounts
    exported_mounts = [m for m in resolve_network_storage() if is_controller_mount(m)]

    # key by remote mount path since that is what needs exporting
    to_export = {m.remote_mount: "*(rw,no_subtree_check,no_root_squash)" for m in exported_mounts}

    key_mount = lkp.slurm_key_mount if lkp.cfg.enable_slurm_auth else lkp.munge_mount
    if is_controller_mount(key_mount):
        # Export key mount as read-only
        to_export[key_mount.remote_mount] = "*(ro,no_subtree_check,no_root_squash)"
    
    if util.should_mount_slurm_bucket():
        mnt = get_slurm_bucket_mount()
        # FSID is required for virtual filesystem that is not based on a device
        # Also export it as read-only
        fsid=str(uuid.uuid4())
        to_export[mnt.local_mount] = f"*(ro,no_subtree_check,no_root_squash,fsid={fsid})"

    # export path if corresponding selector boolean is True
    lines = []
    for path,options in to_export.items():
        util.mkdirp(Path(path))
        run(rf"sed -i '\#{path}#d' /etc/exports", timeout=30)
        lines.append(f"{path}  {options}")

    exportsd = Path("/etc/exports.d")
    util.mkdirp(exportsd)
    with (exportsd / "slurm.exports").open("w") as f:
        f.write("\n")
        f.write("\n".join(lines))
    run("exportfs -a", timeout=30)
