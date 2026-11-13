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

import argparse
from itertools import chain
import logging
import os
import re
import shlex
import shutil
import subprocess
import stat
import sys
import time
from typing import List
import yaml
from pathlib import Path
import functools
import socket
import grp, pwd

import util
from util import (
    lookup,
    dirs,
    slurmdirs,
    run,
    install_custom_scripts,
)
import conf
import slurmsync

from setup_network_storage import (
    setup_network_storage,
    setup_nfs_exports,
)


log = logging.getLogger()


MOTD_HEADER = """
                                 SSSSSSS
                                SSSSSSSSS
                                SSSSSSSSS
                                SSSSSSSSS
                        SSSS     SSSSSSS     SSSS
                       SSSSSS               SSSSSS
                       SSSSSS    SSSSSSS    SSSSSS
                        SSSS    SSSSSSSSS    SSSS
                SSS             SSSSSSSSS             SSS
               SSSSS    SSSS    SSSSSSSSS    SSSS    SSSSS
                SSS    SSSSSS   SSSSSSSSS   SSSSSS    SSS
                       SSSSSS    SSSSSSS    SSSSSS
                SSS    SSSSSS               SSSSSS    SSS
               SSSSS    SSSS     SSSSSSS     SSSS    SSSSS
          S     SSS             SSSSSSSSS             SSS     S
         SSS            SSSS    SSSSSSSSS    SSSS            SSS
          S     SSS    SSSSSS   SSSSSSSSS   SSSSSS    SSS     S
               SSSSS   SSSSSS   SSSSSSSSS   SSSSSS   SSSSS
          S    SSSSS    SSSS     SSSSSSS     SSSS    SSSSS    S
    S    SSS    SSS                                   SSS    SSS    S
    S     S                                                   S     S
                SSS
                SSS
                SSS
                SSS
 SSSSSSSSSSSS   SSS   SSSS       SSSS    SSSSSSSSS   SSSSSSSSSSSSSSSSSSSS
SSSSSSSSSSSSS   SSS   SSSS       SSSS   SSSSSSSSSS  SSSSSSSSSSSSSSSSSSSSSS
SSSS            SSS   SSSS       SSSS   SSSS        SSSS     SSSS     SSSS
SSSS            SSS   SSSS       SSSS   SSSS        SSSS     SSSS     SSSS
SSSSSSSSSSSS    SSS   SSSS       SSSS   SSSS        SSSS     SSSS     SSSS
 SSSSSSSSSSSS   SSS   SSSS       SSSS   SSSS        SSSS     SSSS     SSSS
         SSSS   SSS   SSSS       SSSS   SSSS        SSSS     SSSS     SSSS
         SSSS   SSS   SSSS       SSSS   SSSS        SSSS     SSSS     SSSS
SSSSSSSSSSSSS   SSS   SSSSSSSSSSSSSSS   SSSS        SSSS     SSSS     SSSS
SSSSSSSSSSSS    SSS    SSSSSSSSSSSSS    SSSS        SSSS     SSSS     SSSS

"""
_MAINTENANCE_SBATCH_SCRIPT_PATH = dirs.custom_scripts / "perform_maintenance.sh"

def start_motd():
    """advise in motd that slurm is currently configuring"""
    wall_msg = "*** Slurm is currently being configured in the background. ***"
    motd_msg = MOTD_HEADER + wall_msg + "\n\n"
    Path("/etc/motd").write_text(motd_msg)
    util.run(f"wall -n '{wall_msg}'", timeout=30)


def end_motd(broadcast=True):
    """modify motd to signal that setup is complete"""
    Path("/etc/motd").write_text(MOTD_HEADER)

    if not broadcast:
        return

    run(
        "wall -n '*** Slurm {} setup complete ***'".format(lookup().instance_role),
        timeout=30,
    )
    if not lookup().is_controller:
        run(
            """wall -n '
/home on the controller was mounted over the existing /home.
Log back in to ensure your home directory is correct.
'""",
            timeout=30,
        )


def failed_motd():
    if lookup().is_hybrid_setup:
        #Do not modify motd for hybrid setup
        return
    """modify motd to signal that setup is failed"""
    wall_msg = f"*** Slurm setup failed! Please view log: {util.get_log_path()} ***"
    motd_msg = MOTD_HEADER + wall_msg + "\n\n"
    Path("/etc/motd").write_text(motd_msg)
    util.run(f"wall -n '{wall_msg}'", timeout=30)


def _startup_script_timeout(lkp: util.Lookup) -> int:
    if lkp.is_controller:
        return lkp.cfg.get("controller_startup_scripts_timeout", 300)
    elif lkp.instance_role == "compute":
        return lkp.cfg.get("compute_startup_scripts_timeout", 300)
    elif lkp.is_login_node:
        return lkp.cfg.login_groups[util.instance_login_group()].get("startup_scripts_timeout", 300)
    return 300


def run_custom_scripts():
    """run custom scripts based on instance_role"""
    custom_dir = dirs.custom_scripts
    if lookup().is_controller:
        # controller has all scripts, but only runs controller.d
        custom_dirs = [custom_dir / "controller.d"]
    elif lookup().instance_role == "compute":
        # compute setup with nodeset.d
        custom_dirs = [custom_dir / "nodeset.d"]
    elif lookup().is_login_node:
        # login setup with only login.d
        custom_dirs = [custom_dir / "login.d"]
    else:
        # Unknown role: run nothing
        custom_dirs = []

    timeout = _startup_script_timeout(lookup())

    custom_scripts = [
        p
        for d in custom_dirs
        for p in d.rglob("*")
        if p.is_file() and not p.name.endswith(".disabled")
    ]
    print_scripts = ",".join(str(s.relative_to(custom_dir)) for s in custom_scripts)
    log.debug(f"custom scripts to run: {custom_dir}/({print_scripts})")

    try:
        for script in custom_scripts:
            log.info(f"running script {script.name} with timeout={timeout}")
            result = run(str(script), timeout=timeout, check=False, shell=True)
            runlog = (
                f"{script.name} returncode={result.returncode}\n"
                f"stdout={result.stdout}stderr={result.stderr}"
            )
            log.info(runlog)
            result.check_returncode()
    except OSError as e:
        log.error(f"script {script} is not executable")
        raise e
    except subprocess.TimeoutExpired as e:
        log.error(f"script {script} did not complete within timeout={timeout}")
        raise e
    except Exception as e:
        log.exception(f"script {script} encountered an exception")
        raise e

def mount_save_state_disk():
    disk_name = f"/dev/disk/by-id/google-{lookup().cfg.controller_state_disk.device_name}"
    mount_point = util.slurmdirs.state
    fs_type = "ext4"

    rdevice = util.run(f"realpath {disk_name}").stdout.strip()
    file_output = util.run(f"file -s {rdevice}").stdout.strip()
    if "filesystem" not in file_output:
        util.run(f"mkfs -t {fs_type} -q {rdevice}")

    fstab_entry = f"{disk_name} {mount_point} {fs_type}"
    with open("/etc/fstab", "r") as f:
        fstab = f.readlines()
    if fstab_entry not in fstab:
        with open("/etc/fstab", "a") as f:
            f.write(f"{fstab_entry} defaults 0 0\n")

    util.run(f"systemctl daemon-reload")

    os.makedirs(mount_point, exist_ok=True)
    if util.run(f"mountpoint -q {mount_point}", check=False).returncode == 0:
        log.info(f"Mount point {mount_point} is already mounted. Skipping mount.")
    else:
        util.run(f"mount {mount_point}")

    util.chown_slurm(mount_point)


def setup_jwt_key():
    jwt_key = Path(slurmdirs.state / "jwt_hs256.key")

    if jwt_key.exists():
        log.info("JWT key already exists. Skipping key generation.")
    else:
        run("dd if=/dev/urandom bs=32 count=1 > " + str(jwt_key), shell=True)

    util.chown_slurm(jwt_key, mode=0o400)


def _generate_key(p: Path) -> None:
    run(f"dd if=/dev/random of={p} bs=1024 count=1")


def setup_key(lkp: util.Lookup, is_primary: bool = True) -> None:
    file_name = "munge.key"
    dir = dirs.munge

    if lkp.cfg.enable_slurm_auth:
      file_name = "slurm.key"
      dir = slurmdirs.etc

    dst = Path(dir / file_name)

    shared_dir = Path(slurmdirs.state)
    shared_key = shared_dir / file_name

    if is_primary:
        # Primary Controller Logic:
        # 1. Restore  key from shared state if local is missing but shared exists
        # 2. Generate local if neither exists
        # 3. Sync local to shared

        if not dst.exists():
            if shared_key.exists():
                log.info(f"Restoring {file_name} from {shared_key}")
                shutil.copyfile(shared_key, dst)
            elif lkp.cfg.controller_state_disk and lkp.cfg.controller_state_disk.device_name:
                 # For zonal state disk (files might be directly there if mounted at slurmdirs.state)
                 # But if slurmdirs.state IS shared_dir, we already checked shared_key.exists()
                 # If controller_state_disk is used, slurmdirs.state IS the persistence.
                 pass

            if not dst.exists(): # Still missing
                log.info(f"Generating {file_name}")
                _generate_key(dst)

        # Set permissions on local key
        if lkp.cfg.enable_slurm_auth:
            util.chown_slurm(dst, mode=0o400)
        else:
            shutil.chown(dst, user="munge", group="munge")
            os.chmod(dst, stat.S_IRUSR)

        # Sync to shared location
        if not shared_key.exists():
            log.info(f"Syncing {file_name} to {shared_key}")
            try:
                shutil.copyfile(dst, shared_key)
                if lkp.cfg.enable_slurm_auth:
                    util.chown_slurm(shared_key, mode=0o400)
                else:
                    util.chown_slurm(shared_key, mode=0o600)
            except Exception as e:
                log.warning(f"Failed to sync key to shared storage: {e}")

    else:
        # Wait for key to appear in shared storage and copy to local destination

        timeout = lkp.cfg.get("backup_controller_key_timeout", 300)
        snooze = 10 # seconds
        retries = int(timeout / snooze)
        log.info(f"Waiting for {file_name} in {shared_dir} for up to {timeout} seconds")
        for _ in range(retries):
            if shared_key.exists():
                break
            time.sleep(snooze)

        if not shared_key.exists():
            log.error(f"Timed out waiting for {file_name} in {shared_dir}")
            raise FileNotFoundError(f"{file_name} not found in shared storage")

        log.info(f"Copying {file_name} from {shared_dir}")
        shutil.copyfile(shared_key, dst)

        if lkp.cfg.enable_slurm_auth:
            util.chown_slurm(dst, mode=0o400)
        else:
            shutil.chown(dst, user="munge", group="munge")
            os.chmod(dst, stat.S_IRUSR)

    if lkp.cfg.enable_slurm_auth:
        # Put key into shared volume for distribution
        if is_primary:
            util.mkdirp(util.slurmdirs.key_distribution)
            util.chown_slurm(util.slurmdirs.key_distribution, mode=0o755)

            distributed = util.slurmdirs.key_distribution / file_name
            shutil.copyfile(dst, distributed)
            util.chown_slurm(distributed, mode=0o400)
        # Munge is distributed from /etc/munge.
    else:
        run("systemctl restart munge", timeout=30)


def setup_nss_slurm():
    """install and configure nss_slurm"""
    # setup nss_slurm
    util.mkdirp(Path("/var/spool/slurmd"))
    run(
        "ln -s {}/lib/libnss_slurm.so.2 /usr/lib64/libnss_slurm.so.2".format(
            slurmdirs.prefix
        ),
        check=False,
    )
    run(r"sed -i 's/\(^\(passwd\|group\):\s\+\)/\1slurm /g' /etc/nsswitch.conf")


def setup_sudoers():
    lines = [
        "# Allow SlurmUser to manage the slurm daemons",
        "slurm ALL= NOPASSWD: /usr/bin/systemctl restart slurmd.service",
        "slurm ALL= NOPASSWD: /usr/bin/systemctl restart sackd.service",
        "slurm ALL= NOPASSWD: /usr/bin/systemctl restart slurmctld.service",
    ]
    if util.lookup().is_controller:
        lines.extend([
            "# Allow SlurmUser on controllers to adjust nfsd thread pool without option injection",
            "slurm ALL= NOPASSWD: /usr/sbin/nfsconf --set -- nfsd threads [1-9]*, !/usr/sbin/nfsconf --set -- nfsd threads * *",
            "slurm ALL= NOPASSWD: /usr/sbin/rpc.nfsd -- [1-9]*, !/usr/sbin/rpc.nfsd -- * *",
        ])
    content = "\n" + "\n".join(lines) + "\n"
    sudoers_file = Path("/etc/sudoers.d/slurm")
    sudoers_file.write_text(content)
    sudoers_file.chmod(0o0440)


_SCALE_SYSCTL_CONTENT = """# Managed by Slurm GCP setup.py for high-scale RPC & metadata traffic
net.ipv4.ip_local_port_range = 10240 65535
net.ipv4.tcp_tw_reuse = 1
net.core.somaxconn = 16384
net.ipv4.tcp_max_syn_backlog = 16384
"""

_SCALE_LIMITS_CONTENT = """# Managed by Slurm GCP setup.py for large-scale srun fan-out
* soft nofile 131072
* hard nofile 131072
root soft nofile 131072
root hard nofile 131072
"""


_CONTROLLER_SERVICE_DROPIN_CONTENT = """# Managed by Slurm GCP setup.py for daemon crash resilience & scale FD limits
[Service]
Restart=on-failure
RestartSec=5s
LimitNOFILE=131072
"""


def configure_controller_systemd_units(systemd_dir: Path = Path("/etc/systemd/system")) -> None:
    """Ensure slurmctld, slurmdbd, and slurmrestd auto-restart on crash/SIGKILL and have 131072 FDs."""
    changed = False
    for svc in ("slurmctld.service", "slurmdbd.service", "slurmrestd.service"):
        dropin = systemd_dir / f"{svc}.d" / "90-slurm-scale.conf"
        util.mkdirp(dropin.parent)
        if not dropin.exists() or dropin.read_text() != _CONTROLLER_SERVICE_DROPIN_CONTENT:
            dropin.write_text(_CONTROLLER_SERVICE_DROPIN_CONTENT)
            changed = True
    if changed:
        run("systemctl daemon-reload", timeout=30, check=False)


def _rewrite_nscd_conf_text(content: str) -> str:
    """Enable passwd/group caching in nscd while keeping hosts caching disabled for cloud_dns."""
    desired = {
        "passwd": "yes",
        "group": "yes",
        "hosts": "no",
        "services": "no",
        "netgroup": "no",
    }
    seen = set()
    out_lines = []
    pattern = re.compile(r"^(\s*enable-cache\s+)(\S+)(\s+)\S+(.*)$")
    for line in content.splitlines():
        m = pattern.match(line)
        if m and m.group(2) in desired:
            svc = m.group(2)
            seen.add(svc)
            out_lines.append(f"{m.group(1)}{svc}{m.group(3)}{desired[svc]}{m.group(4)}")
        else:
            out_lines.append(line)
    for svc, val in desired.items():
        if svc not in seen:
            out_lines.append(f"\tenable-cache\t\t{svc}\t\t{val}")
    return "\n".join(out_lines) + "\n"


def configure_scale_limits_and_sysctls(
    sysctl_path: Path = Path("/etc/sysctl.d/90-slurm-scale.conf"),
    limits_path: Path = Path("/etc/security/limits.d/90-slurm-scale.conf"),
) -> None:
    """Apply TCP listen/ephemeral-port sysctls and PAM nofile limits for large-scale Slurm fan-out."""
    try:
        util.mkdirp(sysctl_path.parent)
        sysctl_path.write_text(_SCALE_SYSCTL_CONTENT)
        run(f"sysctl -p {shlex.quote(str(sysctl_path))}", timeout=15, check=False)
    except Exception:
        log.exception(f"Failed to apply scale sysctls at {sysctl_path}")

    try:
        util.mkdirp(limits_path.parent)
        limits_path.write_text(_SCALE_LIMITS_CONTENT)
    except Exception:
        log.exception(f"Failed to write PAM limits at {limits_path}")


def configure_scale_network_and_nss(
    sysctl_path: Path = Path("/etc/sysctl.d/90-slurm-scale.conf"),
    nscd_conf: Path = Path("/etc/nscd.conf"),
    limits_path: Path = Path("/etc/security/limits.d/90-slurm-scale.conf"),
) -> None:
    """Tune ephemeral port reuse and enable OS Login / NSS passwd+group caching on control/login nodes.

    Without nscd or google-oslogin-cache active, every slurm_cred_create() fetch_identity() call
    and Slurm CLI UID lookup opens an unpooled HTTP connection via libnss_oslogin to
    169.254.169.254:80, exhausting ephemeral ports in TIME_WAIT (~28k sockets) at >5k-node scale.
    On Rocky 9 / Ubuntu / Debian where nscd is absent, google-oslogin-cache.service populates
    /etc/oslogin_passwd.cache.bin (served by libnss_cache_oslogin) while tcp_tw_reuse=1 and the
    widened ephemeral port range protect uncached lookups.
    """
    configure_scale_limits_and_sysctls(sysctl_path=sysctl_path, limits_path=limits_path)

    # Populate /etc/oslogin_passwd.cache and keep the refresh timer enabled when available.
    run("systemctl enable --now google-oslogin-cache.timer", timeout=30, check=False)
    run("systemctl start google-oslogin-cache.service", timeout=60, check=False)

    # Enable nscd strictly for passwd/group (never hosts, so cloud_dns never caches negative NXDOMAIN).
    if nscd_conf.exists():
        try:
            nscd_conf.write_text(_rewrite_nscd_conf_text(nscd_conf.read_text()))
            run("systemctl enable --now nscd", timeout=30, check=False)
            run("systemctl restart nscd", timeout=30, check=False)
        except Exception:
            log.exception("Failed to configure and start nscd")


def setup_maintenance_script():
    perform_maintenance = """#!/bin/bash

#SBATCH --priority=low
#SBATCH --time=180

VM_NAME=$(curl -s "http://metadata.google.internal/computeMetadata/v1/instance/name" -H "Metadata-Flavor: Google")
ZONE=$(curl -s "http://metadata.google.internal/computeMetadata/v1/instance/zone" -H "Metadata-Flavor: Google" | cut -d '/' -f 4)

gcloud compute instances perform-maintenance $VM_NAME \
  --zone=$ZONE
"""


    with open(_MAINTENANCE_SBATCH_SCRIPT_PATH, "w") as f:
        f.write(perform_maintenance)

    util.chown_slurm(_MAINTENANCE_SBATCH_SCRIPT_PATH, mode=0o755)


def update_system_config(file, content):
    """Add system defaults options for service files"""
    sysconfig = Path("/etc/sysconfig")
    default = Path("/etc/default")

    if sysconfig.exists():
        conf_dir = sysconfig
    elif default.exists():
        conf_dir = default
    else:
        raise Exception("Cannot determine system configuration directory.")

    slurmd_file = Path(conf_dir, file)
    slurmd_file.write_text(content)

def _symlink_mysql_datadir(lkp: util.Lookup) -> None:
    """ Symlink /var/lib/mysql to controller state disk if needed. """
    if not lkp.cfg.controller_state_disk or not lkp.cfg.controller_state_disk.device_name:
        return
    
    datadir = Path("/var/lib/mysql")
    dst = slurmdirs.state / "mysql"

    if dst.exists():
        run(f"rm -rf {datadir}")
    else:
        shutil.move(datadir, dst)    

    datadir.symlink_to(dst, target_is_directory=True)
    shutil.chown(datadir, user="mysql", group="mysql")
    run(f"chown -R mysql:mysql {dst}")

def configure_mysql(lkp: util.Lookup) -> None:
    cnfdir = Path("/etc/my.cnf.d")
    if not cnfdir.exists():
        cnfdir = Path("/etc/mysql/conf.d")
    if not (cnfdir / "mysql_slurm.cnf").exists():
        (cnfdir / "mysql_slurm.cnf").write_text(
            """
[mysqld]
bind-address=127.0.0.1
innodb_buffer_pool_size=1024M
innodb_log_file_size=64M
innodb_lock_wait_timeout=900
"""
        )

    run("systemctl stop mariadb", timeout=30)
    _symlink_mysql_datadir(lkp)

    run("systemctl enable mariadb", timeout=30)
    run("systemctl restart mariadb", timeout=30)
    
    db_name = "slurm_acct_db"
    

    cmd = "mysql -u root -e"
    for host  in ("localhost", lkp.control_host):
        run(f"""{cmd} "drop user if exists 'slurm'@'{host}'";""", timeout=30)
        run(f"""{cmd} "create user 'slurm'@'{host}'";""", timeout=30)
        run(f"""{cmd} "grant all on {db_name}.* TO 'slurm'@'{host}'";""", timeout=30)


def widen_event_cluster_nodes(lkp: util.Lookup) -> None:
    """Store the full cluster node list in the slurmdbd event table."""
    # slurmdbd keeps the cluster hostlist in a TEXT column. Past 64 KiB it is truncated, never
    # matches on the next registration and slurmdbd re-logs every node every 5 minutes.
    table = f"{lkp.cfg.slurm_cluster_name}_event_table"
    query = (
        "select data_type from information_schema.columns where table_schema='slurm_acct_db'"
        f" and table_name='{table}' and column_name='cluster_nodes'"
    )
    alter = f"alter table `{table}` modify `cluster_nodes` mediumtext not null default ''"
    # Best effort, a failed ALTER must not keep slurmctld from starting.
    try:
        data_type = run(f"mysql -N -u root -e {shlex.quote(query)}", timeout=30).stdout.strip()
        if data_type != "text":
            log.info(f"{table}.cluster_nodes is '{data_type or 'missing'}', not widening")
            return
        log.info(f"Widening {table}.cluster_nodes to mediumtext")
        run(f"mysql -u root slurm_acct_db -e {shlex.quote(alter)}", timeout=600)
    except Exception:
        log.exception(f"Failed to widen {table}.cluster_nodes")


def configure_dirs():
    for p in dirs.values():
        util.mkdirp(p)

    for p in (dirs.slurm, dirs.scripts, dirs.custom_scripts):
        util.chown_slurm(p)

    for name, path in slurmdirs.items():
        util.mkdirp(path)
        if name != "prefix":
            util.chown_slurm(path)

    for sl, tgt in ( # create symlinks
        (Path("/etc/slurm"), slurmdirs.etc),
        (dirs.scripts / "etc", slurmdirs.etc),
        (dirs.scripts / "log", dirs.log),
    ):
        if sl.exists() and sl.is_symlink():
            sl.unlink()
        sl.symlink_to(tgt)

    # copy auxiliary scripts
    for dst_folder, src_file in ((lookup().cfg.slurm_bin_dir,
                                  Path("sort_nodes.py")),
                                 (dirs.custom_scripts / "task_prolog.d",
                                  Path("tools/task-prolog")),
                                 (dirs.custom_scripts / "task_epilog.d",
                                  Path("tools/task-epilog"))):
        dst = Path(dst_folder) / src_file.name
        util.mkdirp(dst.parent)
        shutil.copyfile(util.scripts_dir / src_file, dst)
        os.chmod(dst, 0o755)


def self_report_controller_address(lkp: util.Lookup) -> None:
    if not lkp.cfg.controller_network_attachment:
        return # only self report address if network attachment is used
    data = { "slurm_control_addr": lkp.cfg.slurm_control_addr }
    bucket, prefix = util._get_bucket_and_common_prefix()
    blob = util.storage_client().bucket(bucket).blob(f"{prefix}/controller_addr.yaml")
    with blob.open('w') as f:
        f.write(yaml.dump(data))

def check_slurmdbd_ready(maxtries: int = 30, max_interval: float = 15.0) -> None:
    """Poll sacctmgr until slurmdbd is ready, with timeout safety and backoff."""
    sacctmgr = slurmdirs.prefix / "bin" / "sacctmgr"
    wait = 10.0
    for try_cnt in range(1, maxtries + 1):
        try:
            res = run(f"{sacctmgr} -n -i show cluster", check=False, timeout=10)
            if res.returncode == 0:
                log.info(f"slurmdbd is ready (try {try_cnt}).")
                return
        except Exception as e:
            log.debug(f"slurmdbd check error: {e}")

        if try_cnt < maxtries:
            log.info(f"Waiting for slurmdbd to be ready... (try {try_cnt}/{maxtries}, retrying in {int(wait)}s)")
            time.sleep(wait)
            wait = min(wait * 1.5, max_interval)

    log.error(f"Slurmdbd is still not ready after {maxtries} tries. Expecting issues!")
    raise RuntimeError("Slurmdbd is not ready")

def check_sackd_ready(maxtries: int = 40, max_interval: float = 15.0) -> None:
    """Poll sackd until it successfully connects to slurmctld and becomes active."""
    run("systemctl enable sackd", timeout=30)
    wait = 10.0
    for try_cnt in range(1, maxtries + 1):
        try:
            proc = run("systemctl restart sackd", timeout=60, check=False)
            if proc.returncode == 0:
                active_proc = run("systemctl is-active sackd", timeout=15, check=False)
                if active_proc.returncode == 0:
                    log.info(f"sackd is ready and active (try {try_cnt}).")
                    return
        except subprocess.TimeoutExpired:
            log.info("Timed out waiting for sackd to start; waiting for controller slurmctld to become ready...")
        except Exception as e:
            log.debug(f"sackd check error: {e}")

        if try_cnt < maxtries:
            log.info(f"Waiting for sackd to be ready... (try {try_cnt}/{maxtries}, retrying in {int(wait)}s)")
            time.sleep(wait)
            wait = min(wait * 1.5, max_interval)

    log.error(f"sackd is still not ready after {maxtries} tries. Expecting issues!")
    run("systemctl status sackd", timeout=30, check=False)


def setup_slurm_health_check_service(lkp: util.Lookup) -> None:
    if not lkp.cfg.get("slurm_backup_controller_name") or not lkp.cfg.get("enable_controller_load_balancer"):
        return
    log.info("Setting up Slurm HA HTTP Health Check agent systemd service")
    src_file = util.scripts_dir / "slurm_health_check.py"
    if not src_file.exists():
        log.warning(f"Health check script {src_file} not found; skipping service setup")
        return

    dst_file = Path("/usr/local/bin/slurm_health_check.py")
    if dst_file.exists():
        try:
            dst_file.unlink()
        except Exception as e:
            log.warning(f"Failed to unlink existing health check script: {e}")
    shutil.copyfile(src_file, dst_file)
    os.chmod(dst_file, 0o755)

    service_content = """[Unit]
Description=Slurm HA HTTP Health Check Agent
After=network.target slurmctld.service

[Service]
Type=simple
ExecStart=/usr/local/bin/slurm_health_check.py
Restart=always
RestartSec=3
User=slurm
Group=slurm

[Install]
WantedBy=multi-user.target
"""
    service_path = Path("/etc/systemd/system/slurm-health-check.service")
    service_path.write_text(service_content)
    os.chmod(service_path, 0o644)

    run("systemctl daemon-reload", timeout=30)
    run("systemctl enable slurm-health-check.service", timeout=30)
    run("systemctl restart slurm-health-check.service", timeout=30)

def setup_controller(is_primary: bool):
    """Shared controller setup logic for both primary and backup."""
    role_name = "Primary" if is_primary else "Backup"
    log.info(f"Setting up {role_name} controller")
    lkp = util.lookup()

    # Topology and configuration
    util.chown_slurm(dirs.scripts / "config.yaml", mode=0o600)
    install_custom_scripts()
    conf.get_generator(lkp).generate_configs()

    setup_sudoers()
    configure_scale_network_and_nss()
    setup_network_storage()

    # Ensure state directory permissions are correct (especially if mounted via NFS)
    util.chown_slurm(slurmdirs.state)

    # Primary-specific state disk mounting
    if is_primary:
        if lkp.cfg.controller_state_disk and lkp.cfg.controller_state_disk.device_name:
            mount_save_state_disk()

    setup_jwt_key()
    setup_key(lkp, is_primary=is_primary)

    run_custom_scripts()

    if not lkp.cfg.cloudsql_secret:
        configure_mysql(lkp)

    if not is_primary:
        db_sentinel = Path(slurmdirs.state / "db_initialized")
        timeout = 300
        snooze = 10
        retries = int(timeout / snooze)
        log.info("Waiting for database initialization to complete on primary...")
        for _ in range(retries):
            if db_sentinel.exists():
                break
            time.sleep(snooze)
        if not db_sentinel.exists():
            log.warning("Timed out waiting for database initialization sentinel on primary. Proceeding anyway.")

    configure_controller_systemd_units()
    run("systemctl enable slurmdbd", timeout=30)
    run("systemctl restart slurmdbd", timeout=30)

    # Wait for slurmdbd to come up.
    # This can take a while in case of database migration after upgrades (up to an hour).
    check_slurmdbd_ready(maxtries=30, max_interval=240)

    sacctmgr = f"{slurmdirs.prefix}/bin/sacctmgr -i"
    result = run(
        f"{sacctmgr} add cluster {lkp.cfg.slurm_cluster_name}", timeout=30, check=False
    )
    if "already exists" in result.stdout:
        log.info(result.stdout)
    elif result.returncode > 1:
        result.check_returncode()  # will raise error

    if not lkp.cfg.cloudsql_secret and is_primary:
        widen_event_cluster_nodes(lkp)

    # Remove clustername file to prevent CLUSTER ID MISMATCH errors when
    # controller is recreated but slurmdbd retains state from previous cluster.
    # slurmctld will get the correct cluster ID from slurmdbd on startup.
    clustername_file = Path(slurmdirs.state / "clustername")
    if clustername_file.exists():
        log.info(f"Removing {clustername_file} to prevent cluster ID mismatch")
        clustername_file.unlink()

    if is_primary:
        db_sentinel = Path(slurmdirs.state / "db_initialized")
        try:
            db_sentinel.touch()
            util.chown_slurm(db_sentinel)
        except Exception as e:
            log.warning(f"Failed to create database initialization sentinel: {e}")

    run("systemctl enable slurmctld", timeout=30)
    run("systemctl restart slurmctld", timeout=30)

    run("systemctl enable slurmrestd", timeout=30)
    run("systemctl restart slurmrestd", timeout=30)

    setup_slurm_health_check_service(lkp)

    # Prepare /etc/exports.d/slurm.exports and /etc/nfs.conf thread count BEFORE starting
    # nfs-server so TCP :2049 never opens with an empty export table or 8 stock threads.
    setup_nfs_exports()
    run("systemctl enable nfs-server", timeout=30)
    run("systemctl start nfs-server", timeout=30)
    run("exportfs -ra", timeout=30)
    run("systemctl enable --now slurmcmd.timer", timeout=30)

    log.info("Check status of cluster services")
    if not lkp.cfg.enable_slurm_auth:
        run("systemctl status munge", timeout=30)
    run("systemctl status slurmdbd", timeout=30)
    run("systemctl status slurmctld", timeout=30)
    run("systemctl status slurmrestd", timeout=30)

    if is_primary:
        try:
            slurmsync.sync_instances()
        except Exception:
            log.exception("Failed to sync instances, will try next time.")

    run("systemctl enable slurm_load_bq.timer", timeout=30)
    run("systemctl start slurm_load_bq.timer", timeout=30)
    run("systemctl status slurm_load_bq.timer", timeout=30)

    # Add script to perform maintenance
    setup_maintenance_script()

    self_report_controller_address(lkp)

    log.info(f"Done setting up {role_name} controller")
    pass

def setup_login():
    """run login node setup"""
    log.info("Setting up login")

    lkp = lookup()
    primary_control_host = lkp.control_host_addr or lkp.control_host
    backup_control_host = lkp.cfg.get("slurm_backup_controller_ip") or lkp.cfg.get("slurm_backup_controller_name")

    servers = [f"{primary_control_host}:{lkp.control_host_port}"]
    if backup_control_host:
        servers.append(f"{backup_control_host}:{lkp.control_host_port}")

    conf_server_arg = ",".join(f'"{s}"' for s in servers)
    sackd_options = [f"--conf-server={conf_server_arg}"]
    sysconf = f"""SACKD_OPTIONS='{" ".join(sackd_options)}'"""
    update_system_config("sackd", sysconf)
    install_custom_scripts()

    setup_network_storage()
    setup_sudoers()
    configure_scale_network_and_nss()
    if not lkp.cfg.enable_slurm_auth:
      run("systemctl restart munge", timeout=30)
    check_sackd_ready()
    run("systemctl enable --now slurmcmd.timer", timeout=30)

    run_custom_scripts()

    log.info("Check status of cluster services")
    if not lkp.cfg.enable_slurm_auth:
      run("systemctl status munge", timeout=30)
    run("systemctl status sackd", timeout=30)

    log.info("Done setting up login")

def _cond_chown(path: Path, uid: int, gid: int, previous_uid: int = -1, previous_gid: int = -1):
    """
    If the current owner of the file at path is previous_uid change it to uid.
    The same goes for the group.
    """
    try:
        st = path.lstat()
    except FileNotFoundError:
        return
    my_uid = uid if (previous_uid == -1 or st.st_uid == previous_uid) else -1
    my_gid = gid if (previous_gid == -1 or st.st_gid == previous_gid) else -1
    if my_uid != -1 or my_gid != -1:
        try:
            os.lchown(path, my_uid, my_gid)
        except OSError:
            pass

def _same_device(path: Path, dev: int) -> bool:
    try:
        return path.lstat().st_dev == dev
    except OSError:
        return False

def recursive_chown(path: Path, uid: int, gid: int, previous_uid: int = -1, previous_gid: int = -1):
    """
    This is the python equivalent of doing:
    find path -xdev -uid previous_uid -exec chown uid {} +
    find path -xdev -uid previous_gid -exec chgrp gid {} +
    """
    try:
        dev = path.lstat().st_dev
    except OSError:
        return

    # Change ownership for the root directory
    _cond_chown(path, uid, gid, previous_uid, previous_gid)

    for root, dirs, files in os.walk(path):
        # Staying on one filesystem avoids chowning shared network mounts (e.g. an
        # on-prem NFS export mounted under /slurm) in hybrid deployments.
        dirs[:] = [d for d in dirs if _same_device(Path(root) / d, dev)]
        for item in chain(dirs,files):
            _cond_chown(Path(os.path.join(root, item)), uid, gid, previous_uid, previous_gid)


def change_uid_gid_slurm():
    lkp = lookup()
    need_chown = False
    cur_gid = grp.getgrnam("slurm").gr_gid
    gid = lkp.cfg.hybrid_conf.slurm_gid
    if (cur_gid != gid):
        need_chown = True
        grc = run(f"groupmod -g {gid} slurm")
        if grc.returncode:
            raise RuntimeError(f"Cannot change the gid of slurm rc={grc.returncode} stdout={grc.stdout} stderr={grc.stderr}")

    cur_uid = pwd.getpwnam("slurm").pw_uid
    uid = lkp.cfg.hybrid_conf.slurm_uid
    if (cur_uid != uid):
        need_chown = True
        urc = run(f"usermod -u {uid} slurm")
        if urc.returncode:
            raise RuntimeError(f"Cannot change the uid of slurm rc={urc.returncode} stdout={urc.stdout} stderr={urc.stderr}")
    if need_chown:
        recursive_chown(slurmdirs.home, uid, gid)
        recursive_chown(slurmdirs.etc, uid, gid, cur_uid, cur_gid)
        recursive_chown(Path(lkp.cfg.slurm_log_dir), uid, gid, cur_uid, cur_gid)
        recursive_chown(dirs.slurm, uid, gid, cur_uid, cur_gid) #/slurm

def setup_compute():
    """run compute node setup"""
    log.info("Setting up compute")

    lkp = lookup()
    if lkp.cfg.hybrid:
        change_uid_gid_slurm()
    util.chown_slurm(dirs.scripts / "config.yaml", mode=0o600)
    primary_control_host = lkp.control_host_addr or lkp.control_host
    backup_control_host = lkp.cfg.get("slurm_backup_controller_ip") or lkp.cfg.get("slurm_backup_controller_name")

    servers = [f"{primary_control_host}:{lkp.control_host_port}"]
    if backup_control_host:
        servers.append(f"{backup_control_host}:{lkp.control_host_port}")

    conf_server_arg = ",".join(f'"{s}"' for s in servers)
    slurmd_options = [f"--conf-server={conf_server_arg}"]

    try:
        slurmd_feature = util.instance_metadata("attributes/slurmd_feature", silent=True)
    except util.MetadataNotFoundError:
        slurmd_feature = None

    if slurmd_feature is not None:
        slurmd_options.append(f'--conf="Feature={slurmd_feature}"')
        slurmd_options.append("-Z")

    sysconf = f"""SLURMD_OPTIONS='{" ".join(slurmd_options)}'"""
    update_system_config("slurmd", sysconf)
    configure_scale_limits_and_sysctls()
    install_custom_scripts()

    setup_nss_slurm()
    setup_network_storage()

    has_gpu = run("lspci | grep --ignore-case 'NVIDIA' | wc -l", shell=True).returncode
    if has_gpu:
        run("nvidia-smi")

    run_custom_scripts()

    setup_sudoers()
    if not lkp.cfg.enable_slurm_auth:
      run("systemctl restart munge", timeout=30)
    run("systemctl enable slurmd", timeout=30)
    run("systemctl restart slurmd", timeout=30)
    run("systemctl enable --now slurmcmd.timer", timeout=30)

    log.info("Check status of cluster services")
    if not lkp.cfg.enable_slurm_auth:
      run("systemctl status munge", timeout=30)
    run("systemctl status slurmd", timeout=30)

    log.info("Done setting up compute")

def setup_cloud_ops() -> None:
    """Add health checks, deployment info, and updated setup path to cloud ops config."""
    cloudOpsStatus = run(
        "systemctl is-active --quiet google-cloud-ops-agent.service", check=False
    ).returncode

    if cloudOpsStatus != 0:
        return

    with open("/etc/google-cloud-ops-agent/config.yaml", "r") as f:
        file = yaml.safe_load(f)

    # Guard: skip if config is empty or missing the expected slurm receivers structure.
    # This happens when the ops agent is installed with a default empty config (comments only).
    if (
        not isinstance(file, dict)
        or not isinstance(logging := file.get("logging"), dict)
        or not isinstance(receivers := logging.get("receivers"), dict)
        or "setup" not in receivers
    ):
        log.warning(
            "google-cloud-ops-agent config is missing expected structure, skipping cloud ops customization"
        )
        return

    # Update setup receiver path
    file["logging"]["receivers"]["setup"]["include_paths"] = ["/var/log/slurm/setup.log"]

    cluster_info = {
        'type':'modify_fields',
        'fields': {
            'labels."cluster_name"':{
                'static_value':f"{lookup().cfg.slurm_cluster_name}"
            },
            'labels."hostname"':{
                'static_value': f"{lookup().hostname}"
            }
        }
    }

    file["logging"]["processors"]["add_cluster_info"] = cluster_info
    file["logging"]["service"]["pipelines"]["slurmlog_pipeline"]["processors"].append("add_cluster_info")
    file["logging"]["service"]["pipelines"]["slurmlog2_pipeline"]["processors"].append("add_cluster_info")

    lkp = lookup()
    enable_openmetrics = lkp.cfg.get("enable_openmetrics", False)

    if lkp.is_controller and enable_openmetrics:
        if util.slurm_version_gte(lkp.slurm_version, "25.11"):
            # Safely initialize nested dictionaries to prevent KeyErrors from missing/null YAML blocks.
            if not isinstance(file.get("metrics"), dict): file["metrics"] = {}
            metrics = file["metrics"]

            if not isinstance(metrics.get("receivers"), dict): metrics["receivers"] = {}

            if not isinstance(metrics.get("service"), dict): metrics["service"] = {}
            service = metrics["service"]

            if not isinstance(service.get("pipelines"), dict): service["pipelines"] = {}
            pipelines = service["pipelines"]

            if not isinstance(pipelines.get("prometheus_pipeline"), dict): pipelines["prometheus_pipeline"] = {}
            prom_pipeline = pipelines["prometheus_pipeline"]

            if not isinstance(prom_pipeline.get("receivers"), list): prom_pipeline["receivers"] = []
            if not isinstance(prom_pipeline.get("exporters"), list): prom_pipeline["exporters"] = ["google"]
                
            # The Slurm controller natively exposes OpenMetrics on the control host port.
            raw_port = lkp.cfg.get("slurm_control_host_port") or "6818"
            port = str(raw_port).split('-')[0]
            
            # The root /metrics endpoint returns an index page that the Ops Agent cannot parse.
            # We must explicitly define scrape configs for each sub-path.
            # Note: /metrics/jobs-users-accts is intentionally omitted to avoid excessive metric volume.
            metrics["receivers"]["slurm_prometheus"] = {
                "type": "prometheus",
                "config": {
                    "scrape_configs": [
                        {
                            "job_name": "slurm-controller-jobs",
                            "metrics_path": "/metrics/jobs",
                            "scrape_interval": "60s",
                            "static_configs": [{"targets": [f"localhost:{port}"]}]
                        },
                        {
                            "job_name": "slurm-controller-nodes",
                            "metrics_path": "/metrics/nodes",
                            "scrape_interval": "60s",
                            "static_configs": [{"targets": [f"localhost:{port}"]}]
                        },
                        {
                            "job_name": "slurm-controller-partitions",
                            "metrics_path": "/metrics/partitions",
                            "scrape_interval": "120s",
                            "static_configs": [{"targets": [f"localhost:{port}"]}]
                        },
                        {
                            "job_name": "slurm-controller-scheduler",
                            "metrics_path": "/metrics/scheduler",
                            "scrape_interval": "60s",
                            "static_configs": [{"targets": [f"localhost:{port}"]}]
                        }
                    ]
                }
            }
            
            # Append the receiver to the pipeline to activate it, preserving any pre-existing receivers.
            if "slurm_prometheus" not in prom_pipeline["receivers"]:
                prom_pipeline["receivers"].append("slurm_prometheus")

        else:
            log.warning(
                f"enable_openmetrics is enabled, but native OpenMetrics telemetry "
                f"is not supported on Slurm {lkp.slurm_version} (requires >= 25.11)."
            )

    with open("/etc/google-cloud-ops-agent/config.yaml", "w") as f:
        yaml.safe_dump(file, f, sort_keys=False)

    retries = 2
    for _ in range(retries):
        try:
            run("systemctl restart google-cloud-ops-agent.service", timeout=120)
            break
        except subprocess.TimeoutExpired:
            log.error("google-cloud-ops-agent.service did not restart within 120s.")
            result=run("cat /var/log/google-cloud-ops-agent/subagents/logging-module.log", timeout=120, shell=True)
            if result.stdout:
                log.error(f"Logs for google-cloud-ops-agent (logging-module.log file):\n{result.stdout}")
            raise


def get_config(bucket = None):
    sleep_seconds = 5
    while True:
        try:
            if bucket is not None:
                lookup().hybrid_setup = True
            _, cfg = util.fetch_config(bucket=bucket)
            util.update_config(cfg)
            if bucket is not None:
                lookup().hybrid_setup = True
            break
        except util.DeffetiveStoredConfigError as e:
            # install_hybrid.sh runs this by hand, so a bad bucket or missing
            # credentials has to surface instead of retrying forever.
            if bucket is not None:
                raise
            log.warning(f"config is not ready yet: {e}, sleeping for {sleep_seconds}s")
        except Exception:
            if bucket is not None:
                raise
            log.exception(f"unexpected error while fetching config, sleeping for {sleep_seconds}s")
        time.sleep(sleep_seconds)
    log.info("Config fetched")


def _cluster_zones(lkp: util.Lookup) -> List[str]:
    """Return ordered unique GCE zones participating in the cluster."""
    ordered: List[str] = []
    seen = set()

    def _add(raw_zone) -> None:
        z = util.to_leaf_name(raw_zone)
        if z and z not in seen:
            seen.add(z)
            ordered.append(z)

    _add(lkp.cfg.get("controller_zone"))
    _add(lkp.cfg.get("backup_controller_zone"))

    extra_zones = set()
    for ns in chain(
        (lkp.cfg.get("nodeset") or {}).values(),
        (lkp.cfg.get("nodeset_tpu") or {}).values(),
        (lkp.cfg.get("nodeset_dyn") or {}).values(),
        (lkp.cfg.get("login_groups") or {}).values(),
    ):
        if ns is None:
            continue
        getter = ns.get if isinstance(ns, dict) else lambda k: getattr(ns, k, None)
        for key in ("zone_policy_allow", "zones"):
            vals = getter(key)
            if vals:
                for v in vals:
                    z = util.to_leaf_name(v)
                    if z:
                        extra_zones.add(z)
        single = util.to_leaf_name(getter("zone"))
        if single:
            extra_zones.add(single)

    for z in sorted(extra_zones, key=lambda z: ("-ai" in z, z)):
        _add(z)
    return ordered


def _splice_search_domains(existing_tokens: List[str], desired_domains: List[str]) -> List[str]:
    """Merge desired_domains into existing_tokens in deterministic order before google.internal."""
    tokens = list(dict.fromkeys(existing_tokens))
    to_add = [d for d in desired_domains if d not in tokens]
    if not to_add:
        return tokens
    if "google.internal" in tokens:
        idx = tokens.index("google.internal")
        return tokens[:idx] + to_add + tokens[idx:]
    return tokens + to_add


def _merge_resolv_search_lines(lines: List[str], missing_domains: List[str]) -> List[str]:
    """Insert missing_domains into /etc/resolv.conf search directive (before google.internal)."""
    out: List[str] = []
    merged = False
    for line in lines:
        parts = line.strip().split()
        if not merged and parts[:1] == ["search"]:
            tokens = _splice_search_domains(parts[1:], missing_domains)
            out.append("search " + " ".join(tokens))
            merged = True
        else:
            out.append(line)
    if not merged and missing_domains:
        out.insert(0, "search " + " ".join(missing_domains))
    return out


def configure_cluster_dns_search(
    lkp: util.Lookup,
    resolv_path: Path = Path("/etc/resolv.conf"),
) -> None:
    """Ensure multi-region cluster zones are in DNS search domains under ZonalOnly DNS."""
    if lkp.is_hybrid_setup or not lkp.project or not resolv_path.exists():
        return

    zones = _cluster_zones(lkp)
    if not zones:
        return

    try:
        lines = resolv_path.read_text().splitlines()
    except Exception as e:
        log.warning(f"Failed to read {resolv_path}: {e}")
        return

    existing: List[str] = []
    for line in lines:
        parts = line.strip().split()
        if parts[:1] == ["search"]:
            for tok in parts[1:]:
                if tok not in existing:
                    existing.append(tok)

    desired = [f"{z}.c.{lkp.project}.internal" for z in zones]
    missing = [d for d in desired if d not in existing]
    if not missing:
        return

    log.info(f"Adding cross-region cluster DNS search domains: {missing}")
    default_dev = None
    route_res = run("ip -o -4 route show to default", timeout=5, check=False)
    if route_res.returncode == 0 and route_res.stdout:
        m_dev = re.search(r"\bdev\s+(\S+)", route_res.stdout)
        if m_dev:
            default_dev = m_dev.group(1)

    nm_applied = False
    nm_res = run("nmcli -t -f NAME,DEVICE connection show --active", timeout=10, check=False)
    if nm_res.returncode == 0 and nm_res.stdout:
        candidates = []
        for row in nm_res.stdout.splitlines():
            conn_name, sep, dev = row.rpartition(":")
            if sep and conn_name and dev and dev != "lo":
                candidates.append((conn_name, dev))
        chosen = next(((c, d) for c, d in candidates if d == default_dev), None) or (
            candidates[0] if candidates else None
        )
        if chosen:
            conn_name, dev = chosen
            joined = ",".join(missing)
            mod_res = run(
                f"nmcli connection modify {shlex.quote(conn_name)} +ipv4.dns-search {shlex.quote(joined)}",
                timeout=10,
                check=False,
            )
            reapply_res = run(f"nmcli device reapply {shlex.quote(dev)}", timeout=10, check=False)
            nm_applied = mod_res.returncode == 0 and reapply_res.returncode == 0

    # Fallback: verify /etc/resolv.conf contains the domains even if NetworkManager is not managing it
    # (on Ubuntu/Debian with systemd-resolved, configure a resolved.conf.d drop-in instead of overwriting the symlink).
    try:
        if resolv_path.is_symlink() and "systemd" in str(os.readlink(resolv_path)):
            if not nm_applied:
                spliced = _splice_search_domains(existing, desired)
                if len(spliced) > 32 and "google.internal" in spliced:
                    without_gi = [d for d in spliced if d != "google.internal"]
                    merged_domains = without_gi[:31] + ["google.internal"]
                else:
                    merged_domains = spliced[:32]
                dropin = Path("/etc/systemd/resolved.conf.d/90-slurm-scale.conf")
                util.mkdirp(dropin.parent)
                dropin.write_text(f"[Resolve]\nDomains={' '.join(merged_domains)}\n")
                # Restart systemd-resolved BEFORE applying per-link resolvectl domains so the
                # daemon restart does not wipe transient per-interface domain state.
                run("systemctl reload-or-restart systemd-resolved", timeout=15, check=False)
                if default_dev:
                    domains_args = " ".join(shlex.quote(d) for d in merged_domains)
                    run(f"resolvectl domain {shlex.quote(default_dev)} {domains_args}", timeout=10, check=False)
            return
        post_lines = resolv_path.read_text().splitlines()
        post_existing = set()
        for line in post_lines:
            parts = line.strip().split()
            if parts[:1] == ["search"]:
                post_existing.update(parts[1:])
        still_missing = [d for d in missing if d not in post_existing]
        if still_missing:
            updated = _merge_resolv_search_lines(post_lines, still_missing)
            resolv_path.write_text("\n".join(updated) + "\n")
    except Exception as e:
        log.warning(f"Failed to update {resolv_path} fallback search domains: {e}")


def populate_etc_hosts(
    lkp: util.Lookup,
    hosts_path: Path = Path("/etc/hosts"),
) -> None:
    """Populate static IP mappings for controllers in /etc/hosts to bypass DNS propagation latency."""
    log.info("Populating /etc/hosts with controller IP mappings")
    primary_name = lkp.cfg.get("slurm_control_host")
    primary_ip = lkp.cfg.get("slurm_control_addr")
    backup_name = lkp.cfg.get("slurm_backup_controller_name")
    backup_ip = lkp.cfg.get("slurm_backup_controller_ip")

    if lkp.is_controller and lkp.cfg.get("enable_controller_load_balancer"):
        # Controllers must use their real IPs in /etc/hosts, not the Load Balancer VIP
        # This prevents loopback routing loops when backup controller pings primary controller
        log.info("Node is a controller and load balancer is enabled; resolving real IPs for controllers")
        import json
        controller_names = [name for name in (primary_name, backup_name) if name]
        if controller_names:
            names_filter = " ".join(controller_names)
            cmd = ["gcloud", "compute", "instances", "list", f"--project={lkp.project}", f"--filter=name=({names_filter})", "--format=json(name,networkInterfaces[0].networkIP)"]
            try:
                res = subprocess.run(cmd, capture_output=True, text=True, check=True)
                data = json.loads(res.stdout)
                ips = {item['name']: item['networkInterfaces'][0]['networkIP'] for item in data if 'networkInterfaces' in item and item['networkInterfaces']}
                if primary_name in ips:
                    log.info(f"Overriding primary_ip from VIP {primary_ip} to real IP {ips[primary_name]}")
                    primary_ip = ips[primary_name]
                if backup_name in ips:
                    backup_ip = ips[backup_name]
            except Exception as e:
                log.error(f"Failed to query real controller IPs via gcloud: {e}")

    if not hosts_path.exists():
        log.warning("/etc/hosts does not exist, skipping population")
        return

    try:
        lines = hosts_path.read_text().splitlines()
    except Exception as e:
        log.error(f"Failed to read /etc/hosts: {e}")
        return

    names_to_remove = {name for name in (primary_name, backup_name) if name}
    new_lines = []
    for line in lines:
        stripped = line.strip()
        if stripped == "# Added by Slurm HA Setup":
            continue
        if not stripped or stripped.startswith("#"):
            new_lines.append(line)
            continue
        parts = stripped.split()
        if len(parts) > 1 and any(
            any(p == name or p.startswith(f"{name}.") for p in parts[1:])
            for name in names_to_remove
        ):
            continue
        new_lines.append(line)

    new_entries = []
    if primary_ip and primary_name:
        new_entries.append(f"{primary_ip} {primary_name}")
    if backup_ip and backup_name:
        new_entries.append(f"{backup_ip} {backup_name}")

    if new_entries:
        try:
            hosts_path.write_text("\n".join(new_lines) + "\n\n# Added by Slurm HA Setup\n" + "\n".join(new_entries) + "\n")
            log.info(f"Updated /etc/hosts with current controller mappings: {new_entries}")
        except Exception as e:
            log.error(f"Failed to write to /etc/hosts: {e}")


def setup_hybrid(bucket: str):
    log.info("Starting hybrid setup, fetching config")
    get_config(bucket)
    log.info("Generating the config files")
    conf.generate_configs_slurm_v2505(lookup())
    log.info("Success")

def main():
    start_motd()

    log.info("Starting setup, fetching config")
    get_config()
    lkp = lookup()
    configure_cluster_dns_search(lkp)
    populate_etc_hosts(lkp)
    setup_cloud_ops()
    configure_dirs()
    # call the setup function for the instance type
    role = lookup().instance_role
    if role == "controller":
        try:
            ha_role = util.instance_metadata("attributes/slurm_ha_role", silent=True)
            if ha_role == "backup":
                role = "backup_controller"
            elif ha_role == "dynamic":
                hostname = socket.gethostname()
                short_hostname = hostname.split(".")[0]
                if short_hostname.endswith("-0"):
                    log.info(f"Dynamic HA: Hostname '{short_hostname}' ends with '-0'. Assuming Primary role.")
                else:
                    log.info(f"Dynamic HA: Hostname '{short_hostname}' does not end with '-0'. Assuming Standby Backup role.")
                    role = "backup_controller"
        except Exception as e:
            log.warning(f"Failed to read slurm_ha_role metadata: {e}")
            if lkp.cfg.get("slurm_backup_controller_name"):
                hostname = socket.gethostname()
                short_hostname = hostname.split(".")[0]
                if short_hostname.endswith("-0"):
                    log.info(f"Dynamic HA (Fallback): Hostname '{short_hostname}' ends with '-0'. Assuming Primary role.")
                else:
                    log.info(f"Dynamic HA (Fallback): Hostname '{short_hostname}' does not end with '-0'. Assuming Standby Backup role.")
                    role = "backup_controller"

    {
        "controller": lambda: setup_controller(is_primary=True),
        "backup_controller": lambda: setup_controller(is_primary=False),
        "compute": setup_compute,
        "login": setup_login,
    }.get(
        role,
        lambda: log.fatal(f"Unknown node role: {role}"))()

    end_motd()


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--slurmd-feature", dest="slurmd_feature", help="Unused, to be removed.")
    parser.add_argument("--hybrid", dest="hybrid", action="store_true", help="Do the hybrid setup.")
    parser.add_argument("--bucket", dest="bucket", help="The bucket URI where config.yaml is.")
    args = util.init_log_and_parse(parser)

    try:
        if args.hybrid:
            if args.bucket:
                setup_hybrid(args.bucket)
            else:
                log.error("--bucket argument cannot be empty when using --hybrid")
                log.error("Aborting setup...")
                sys.exit(1)
        else:
            main()
    except Exception:
        log.exception("Aborting setup...")
        failed_motd()
