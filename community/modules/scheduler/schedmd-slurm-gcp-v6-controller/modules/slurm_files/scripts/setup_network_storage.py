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

import shutil
from pathlib import Path
from concurrent.futures import as_completed
from addict import Dict as NSDict # type: ignore

import util
from util import NSMount, lookup, run, dirs, separate
from more_executors import Executors, ExceptionRetryPolicy


log = logging.getLogger()

def mounts_by_local(mounts):
    """convert list of mounts to dict of mounts, local_mount as key"""
    return {str(Path(m.local_mount).resolve()): m for m in mounts}


def _get_default_mounts(lkp: util.Lookup) -> List[NSDict]:
    if lkp.cfg.disable_default_mounts:
        return []
    return [
        NSDict(
                server_ip= "$controller",
                remote_mount= path,
                local_mount= path,
                fs_type= "nfs",
                mount_options= "defaults,hard,intr",
        )
        for path in (
            dirs.home,
            dirs.apps,
        )
    ]

def resolve_network_storage(nodeset=None) -> List[NSMount]:
    """Combine appropriate network_storage fields to a single list"""
    lkp = lookup()
    
    # create dict of mounts, local_mount: mount_info
    mounts = mounts_by_local(_get_default_mounts(lkp))

    # On non-controller instances, entries in network_storage could overwrite
    # default exports from the controller. Be careful, of course
    mounts.update(mounts_by_local(lkp.cfg.network_storage))

    if lkp.is_login_node:
        mounts.update(mounts_by_local(lkp.cfg.login_network_storage))

    if lkp.instance_role == "compute":
        try:
            nodeset = lkp.node_nodeset()
        except Exception:
            pass # external nodename, skip lookup
        else:
            mounts.update(mounts_by_local(nodeset.network_storage))

    return [lkp.normalize_ns_mount(mnt) for mnt in mounts.values()]


def separate_external_internal_mounts(mounts):
    """separate into cluster-external and internal mounts"""

    def internal_mount(mount):
        # NOTE: Valid Lustre server_ip can take the form of '<IP>@tcp'
        server_ip = mount.server_ip.split("@")[0]
        mount_addr = util.host_lookup(server_ip)
        return mount_addr == lookup().control_host_addr

    return separate(internal_mount, mounts)


def setup_network_storage():
    """prepare network fs mounts and add them to fstab"""
    log.info("Set up network storage")
    # filter mounts into two dicts, cluster-internal and external mounts

    all_mounts = resolve_network_storage()
    ext_mounts, int_mounts = separate_external_internal_mounts(all_mounts)

    if lookup().is_controller:
        mounts = ext_mounts
    else:
        mounts = ext_mounts + int_mounts

    # Determine fstab entries and write them out
    fstab_entries = []
    for mount in mounts:
        local_mount = Path(mount.local_mount)
        remote_mount = mount.remote_mount
        fs_type = mount.fs_type
        server_ip = mount.server_ip or ""
        util.mkdirp(local_mount)

        log.info(
            "Setting up mount ({}) {}{} to {}".format(
                fs_type,
                server_ip + ":" if fs_type != "gcsfuse" else "",
                remote_mount,
                local_mount,
            )
        )

        mount_options = mount.mount_options.split(",") if mount.mount_options else []
        if not mount_options or "_netdev" not in mount_options:
            mount_options += ["_netdev"]

        if fs_type == "gcsfuse":
            fstab_entries.append(
                "{0}   {1}     {2}     {3}     0 0".format(
                    remote_mount, local_mount, fs_type, ",".join(mount_options)
                )
            )
        else:
            fstab_entries.append(
                "{0}:{1}    {2}     {3}      {4}  0 0".format(
                    server_ip,
                    remote_mount,
                    local_mount,
                    fs_type,
                    ",".join(mount_options),
                )
            )

    fstab = Path("/etc/fstab")
    if not Path(fstab.with_suffix(".bak")).is_file():
        shutil.copy2(fstab, fstab.with_suffix(".bak"))
    shutil.copy2(fstab.with_suffix(".bak"), fstab)
    with open(fstab, "a") as f:
        f.write("\n")
        for entry in fstab_entries:
            f.write(entry)
            f.write("\n")

    mount_fstab(mounts_by_local(mounts), log)
    munge_mount_handler()


def mount_fstab(mounts, log):
    """Wait on each mount, then make sure all fstab is mounted"""
    def mount_path(path):
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
        max_attempts=40, exponent=1.6, sleep=1.0, max_sleep=16.0
    )
    with Executors.thread_pool().with_timeout(MAX_MOUNT_TIMEOUT).with_retry(
        retry_policy=retry_policy
    ) as exe:
        for path in mounts:
            future = exe.submit(mount_path, path)
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
    # wait max 120s for munge mount
    timeout = 120
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


def setup_nfs_exports():
    """nfs export all needed directories"""
    lkp = util.lookup()
    assert lkp.is_controller


    # The controller only needs to set up exports for cluster-internal mounts
    # switch the key to remote mount path since that is what needs exporting
    mounts = resolve_network_storage()
    mounts.append(lkp.munge_mount)
   
    # controller mounts
    _, con_mounts = separate_external_internal_mounts(mounts)
    con_mounts = {m.remote_mount: m for m in con_mounts}
    for nodeset in lkp.cfg.nodeset.values():
        # get internal mounts for each nodeset by calling
        # resolve_network_storage as from a node in each nodeset
        ns_mounts = resolve_network_storage(nodeset=nodeset)
        _, int_mounts = separate_external_internal_mounts(ns_mounts)
        con_mounts.update({m.remote_mount: m for m in int_mounts})

    # export path if corresponding selector boolean is True
    exports = []
    for path in con_mounts:
        util.mkdirp(Path(path))
        run(rf"sed -i '\#{path}#d' /etc/exports", timeout=30)
        exports.append(f"{path}  *(rw,no_subtree_check,no_root_squash)")

    exportsd = Path("/etc/exports.d")
    util.mkdirp(exportsd)
    with (exportsd / "slurm.exports").open("w") as f:
        f.write("\n")
        f.write("\n".join(exports))
    run("exportfs -a", timeout=30)
