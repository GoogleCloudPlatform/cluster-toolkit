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

import argparse
import logging
import os
import shutil
import subprocess
import stat
import time
import yaml
from pathlib import Path
import functools
import base64

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
    util.run(f"mount {mount_point}")

    util.chown_slurm(mount_point)


def setup_jwt_key():
    jwt_key = Path(slurmdirs.state / "jwt_hs256.key")

    if lookup().cfg.jwt_key:
        encoded = util.decrypt(lookup().cfg.kms_key, lookup().cfg.jwt_key)
        with jwt_key.open('wb') as f:
            f.write(base64.b64decode(encoded))
    else:
        util.run("dd if=/dev/urandom bs=32 count=1 >"+str(jwt_key), shell=True)

    util.chown_slurm(jwt_key, mode=0o400)


def _generate_key(p: Path) -> None:

    if lookup().cfg.munge_key:
        encoded = util.decrypt(lookup().cfg.kms_key, lookup().cfg.munge_key)   
        with p.open('wb') as f:
            f.write(base64.b64decode(encoded))
    else:
        run(f"dd if=/dev/random of={p} bs=1024 count=1")


def setup_key(lkp: util.Lookup) -> None:
    file_name = "munge.key"
    dir = dirs.munge

    if lkp.cfg.enable_slurm_auth:
      file_name = "slurm.key"
      dir = slurmdirs.etc

    dst = Path(dir / file_name)

    if lkp.cfg.controller_state_disk.device_name:
        # Copy key from persistent state disk
        persist = slurmdirs.state / file_name
        if not persist.exists():
            _generate_key(persist)

        shutil.copyfile(persist, dst)
        if lkp.cfg.enable_slurm_auth:
            util.chown_slurm(dst, mode=0o400)
            util.chown_slurm(persist, mode=0o400)
        else:
            shutil.chown(dst, user="munge", group="munge")
            os.chmod(dst, stat.S_IRUSR)
    else:
        if dst.exists():
            log.info("key already exists. Skipping key generation.")
        else:
            _generate_key(dst)
            if lkp.cfg.enable_slurm_auth:
              util.chown_slurm(dst, mode=0o400)
            else:
              shutil.chown(dst, user="munge", group="munge")
              os.chmod(dst, stat.S_IRUSR)

    if lkp.cfg.enable_slurm_auth:
        # Put key into shared volume for distribution
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
    content = """
# Allow SlurmUser to manage the slurm daemons
slurm ALL= NOPASSWD: /usr/bin/systemctl restart slurmd.service
slurm ALL= NOPASSWD: /usr/bin/systemctl restart sackd.service
slurm ALL= NOPASSWD: /usr/bin/systemctl restart slurmctld.service
"""
    sudoers_file = Path("/etc/sudoers.d/slurm")
    sudoers_file.write_text(content)
    sudoers_file.chmod(0o0440)


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
    if not lkp.cfg.controller_state_disk.device_name:
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


def configure_dirs():
    for p in dirs.values():
        util.mkdirp(p)

    for p in (dirs.slurm, dirs.scripts, dirs.custom_scripts):
        util.chown_slurm(p)

    for p in slurmdirs.values():
        util.mkdirp(p)
        util.chown_slurm(p)

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

def setup_controller():
    """Run controller setup"""
    log.info("Setting up controller")
    lkp = util.lookup()
    util.chown_slurm(dirs.scripts / "config.yaml", mode=0o600)
    install_custom_scripts()
    conf.gen_controller_configs(lkp)

    if lkp.cfg.controller_state_disk.device_name != None:
        mount_save_state_disk()

    setup_jwt_key()
    setup_key(lkp)

    setup_sudoers()
    setup_network_storage()

    run_custom_scripts()

    if not lkp.cfg.cloudsql_secret:
        configure_mysql(lkp)

    run("systemctl enable slurmdbd", timeout=30)
    run("systemctl restart slurmdbd", timeout=30)

    # Wait for slurmdbd to come up
    time.sleep(5)

    sacctmgr = f"{slurmdirs.prefix}/bin/sacctmgr -i"
    result = run(
        f"{sacctmgr} add cluster {lkp.cfg.slurm_cluster_name}", timeout=30, check=False
    )
    if "already exists" in result.stdout:
        log.info(result.stdout)
    elif result.returncode > 1:
        result.check_returncode()  # will raise error

    run("systemctl enable slurmctld", timeout=30)
    run("systemctl restart slurmctld", timeout=30)

    run("systemctl enable slurmrestd", timeout=30)
    run("systemctl restart slurmrestd", timeout=30)

    # Export at the end to signal that everything is up
    run("systemctl enable nfs-server", timeout=30)
    run("systemctl start nfs-server", timeout=30)

    setup_nfs_exports()
    run("systemctl enable --now slurmcmd.timer", timeout=30)

    log.info("Check status of cluster services")
    if not lkp.cfg.enable_slurm_auth:
      run("systemctl status munge", timeout=30)
    run("systemctl status slurmdbd", timeout=30)
    run("systemctl status slurmctld", timeout=30)
    run("systemctl status slurmrestd", timeout=30)

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

    log.info("Done setting up controller")
    pass


def setup_login():
    """run login node setup"""
    log.info("Setting up login")

    lkp = lookup()
    slurmctld_host = f"{lkp.control_host}"
    if lkp.control_addr:
        slurmctld_host = f"{lkp.control_host}({lkp.control_addr})"
    sackd_options = [
        f'--conf-server="{slurmctld_host}:{lkp.control_host_port}"',
    ]
    sysconf = f"""SACKD_OPTIONS='{" ".join(sackd_options)}'"""
    update_system_config("sackd", sysconf)
    install_custom_scripts()

    setup_network_storage()
    setup_sudoers()
    if not lkp.cfg.enable_slurm_auth:
      run("systemctl restart munge", timeout=30)
    run("systemctl enable sackd", timeout=30)
    run("systemctl restart sackd", timeout=30)
    run("systemctl enable --now slurmcmd.timer", timeout=30)

    run_custom_scripts()

    log.info("Check status of cluster services")
    if not lkp.cfg.enable_slurm_auth:
      run("systemctl status munge", timeout=30)
    run("systemctl status sackd", timeout=30)

    log.info("Done setting up login")


def setup_compute():
    """run compute node setup"""
    log.info("Setting up compute")

    lkp = lookup()
    util.chown_slurm(dirs.scripts / "config.yaml", mode=0o600)
    slurmctld_host = f"{lkp.control_host}"
    if lkp.control_addr:
        slurmctld_host = f"{lkp.control_host}({lkp.control_addr})"
    slurmd_options = [
        f'--conf-server="{slurmctld_host}:{lkp.control_host_port}"',
    ]

    try:
        slurmd_feature = util.instance_metadata("attributes/slurmd_feature", silent=True)
    except util.MetadataNotFoundError:
        slurmd_feature = None

    if slurmd_feature is not None:
        slurmd_options.append(f'--conf="Feature={slurmd_feature}"')
        slurmd_options.append("-Z")

    sysconf = f"""SLURMD_OPTIONS='{" ".join(slurmd_options)}'"""
    update_system_config("slurmd", sysconf)
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

    # Update setup receiver path
    file["logging"]["receivers"]["setup"]["include_paths"] = ["/var/log/slurm/setup.log"]

    # Add chs_health_check receiver
    file["logging"]["receivers"]["chs_health_check"] = {
        "type": "files",
        "include_paths": ["/var/log/slurm/chs_health_check.log"],
        "record_log_file_path": True,
    }

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

    # Add chs_health_check to slurmlog2_pipeline
    file["logging"]["service"]["pipelines"]["slurmlog2_pipeline"]["receivers"].append(
        "chs_health_check"
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


def main():
    start_motd()

    log.info("Starting setup, fetching config")
    sleep_seconds = 5
    while True:
        try:
            _, cfg = util.fetch_config()
            util.update_config(cfg)
            break
        except util.DeffetiveStoredConfigError as e:
            log.warning(f"config is not ready yet: {e}, sleeping for {sleep_seconds}s")
        except Exception as e:
            log.exception(f"unexpected error while fetching config, sleeping for {sleep_seconds}s")
        time.sleep(sleep_seconds)
    log.info("Config fetched")
    setup_cloud_ops()
    configure_dirs()
    # call the setup function for the instance type
    {
        "controller": setup_controller,
        "compute": setup_compute,
        "login": setup_login,
    }.get(
        lookup().instance_role,
        lambda: log.fatal(f"Unknown node role: {lookup().instance_role}"))()

    end_motd()


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--slurmd-feature", dest="slurmd_feature", help="Unused, to be removed.")
    _ = util.init_log_and_parse(parser)

    try:
        main()
    except Exception:
        log.exception("Aborting setup...")
        failed_motd()
