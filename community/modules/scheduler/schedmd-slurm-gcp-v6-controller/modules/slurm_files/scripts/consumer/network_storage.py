
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

import time
import math
import logging
import util
from pathlib import Path
import shutil
from more_executors import Executors, ExceptionRetryPolicy
from concurrent.futures import as_completed

log = logging.getLogger()


def mount_fstab(mounts: list[util.NSMount]) -> None:
    """Wait on each mount, then make sure all fstab is mounted"""
    def mount_path(path: Path):
        log.info(f"Waiting for '{path}' to be mounted...")
        try:
            util.run(f"mount {path}", timeout=120)
        except:
            log.exception(f"mount of path '{path}' failed")
            raise
        log.info(f"Mount point '{path}' was mounted.")

    MAX_MOUNT_TIMEOUT = 60 * 5
    future_list = []
    retry_policy = ExceptionRetryPolicy(
        max_attempts=40, exponent=1.6, sleep=1.0, max_sleep=16.0
    )
    # TODO: Do not rely on `more_executors`, rewrite it using standard library
    with Executors.thread_pool().with_timeout(MAX_MOUNT_TIMEOUT).with_retry(
        retry_policy=retry_policy
    ) as exe:
        for m in mounts:
            future = exe.submit(mount_path, m.local_mount)
            future_list.append(future)

        # Iterate over futures, checking for exceptions
        for future in as_completed(future_list):
            future.result()

def setup_network_storage(cfg: util.Config) -> None:
    """prepare network fs mounts and add them to fstab"""
    log.info("Set up network storage")
    
    # Determine fstab entries and write them out
    fstab_entries = []
    for mount in cfg.network_storage:
        local_mount = mount.local_mount
        fs_type = mount.fs_type
        src = mount.remote_mount if fs_type == "gcsfuse" else f"{mount.server_ip}:{mount.remote_mount}"
        
        log.info(f"Setting up mount ({fs_type}) {src} to {local_mount}")
        local_mount.mkdir(parents=True, exist_ok=True)

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
    mount_fstab(cfg.network_storage)


def slurm_key_mount_handler():
    key_distribution = Path("/slurm/key_distribution")

    mnt = util.NSMount(
        server_ip=util.controller_host(),
        local_mount=key_distribution,
        remote_mount=key_distribution,
        fs_type="nfs",
        mount_options="defaults,hard,intr,_netdev")

    log.info(f"Mounting slurm_key share to: {mnt.local_mount}")
    key_distribution.mkdir(parents=True, exist_ok=True)
    cmd = f"mount --types={mnt.fs_type} --options={mnt.mount_options} {mnt.server_ip}:{mnt.remote_mount} {mnt.local_mount}"
    
    # exponential backoffs 12 retries, first is 0.5s, last 40s, totals 120s
    delays = map(lambda x: math.exp(x / 2.5) / 2, range(12))
    for retry, wait in enumerate(delays, 1):
        try:
            util.run(cmd, timeout=120) # wait max 120s to mount
            break
        except Exception as e:
            log.exception(f"slurm key mount failed, try {retry}, waiting {wait:0.2f}s")
            time.sleep(wait)
            err = e
            continue
    else:
        raise err

    file_name = "slurm.key"
    dst = Path("/usr/local/etc/slurm") / file_name
    log.info(f"Copy slurm.key from: {mnt.local_mount}")
    shutil.copy2(mnt.local_mount / file_name, dst)

    log.debug("Restrict permissions of slurm.key")
    util.chown_slurm(dst, mode=0o400)

    log.debug(f"Unmount {mnt.local_mount}")
    util.run(f"umount {mnt.local_mount}", timeout=120)
    shutil.rmtree(mnt.local_mount)
