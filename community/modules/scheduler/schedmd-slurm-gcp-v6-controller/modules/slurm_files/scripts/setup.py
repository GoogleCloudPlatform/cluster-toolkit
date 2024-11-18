#!/usr/bin/env python3

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

import util
from util import (
    lookup,
    dirs,
    slurmdirs,
    run,
    install_custom_scripts,
)
import conf

from slurmsync import sync_slurm

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


def run_custom_scripts():
    """run custom scripts based on instance_role"""
    custom_dir = dirs.custom_scripts
    if lookup().is_controller:
        # controller has all scripts, but only runs controller.d
        custom_dirs = [custom_dir / "controller.d"]
    elif lookup().instance_role == "compute":
        # compute setup with compute.d and nodeset.d
        custom_dirs = [custom_dir / "compute.d", custom_dir / "nodeset.d"]
    elif lookup().instance_role == "login":
        # login setup with only login.d
        custom_dirs = [custom_dir / "login.d"]
    else:
        # Unknown role: run nothing
        custom_dirs = []
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
            if "/controller.d/" in str(script):
                timeout = lookup().cfg.get("controller_startup_scripts_timeout", 300)
            elif "/compute.d/" in str(script) or "/nodeset.d/" in str(script):
                timeout = lookup().cfg.get("compute_startup_scripts_timeout", 300)
            elif "/login.d/" in str(script):
                timeout = lookup().cfg.get("login_startup_scripts_timeout", 300)
            else:
                timeout = 300
            timeout = None if not timeout or timeout < 0 else timeout
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

def setup_jwt_key():
    jwt_key = Path(slurmdirs.state / "jwt_hs256.key")

    if jwt_key.exists():
        log.info("JWT key already exists. Skipping key generation.")
    else:
        run("dd if=/dev/urandom bs=32 count=1 > " + str(jwt_key), shell=True)

    util.chown_slurm(jwt_key, mode=0o400)


def setup_munge_key():
    munge_key = Path(dirs.munge / "munge.key")

    if munge_key.exists():
        log.info("Munge key already exists. Skipping key generation.")
    else:
        run("create-munge-key -f", timeout=30)

    shutil.chown(munge_key, user="munge", group="munge")
    os.chmod(munge_key, stat.S_IRUSR)
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


def configure_mysql():
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
    run("systemctl enable mariadb", timeout=30)
    run("systemctl restart mariadb", timeout=30)

    mysql = "mysql -u root -e"
    run(f"""{mysql} "drop user 'slurm'@'localhost'";""", timeout=30, check=False)
    run(f"""{mysql} "create user 'slurm'@'localhost'";""", timeout=30)
    run(
        f"""{mysql} "grant all on slurm_acct_db.* TO 'slurm'@'localhost'";""",
        timeout=30,
    )
    run(
        f"""{mysql} "drop user 'slurm'@'{lookup().control_host}'";""",
        timeout=30,
        check=False,
    )
    run(f"""{mysql} "create user 'slurm'@'{lookup().control_host}'";""", timeout=30)
    run(
        f"""{mysql} "grant all on slurm_acct_db.* TO 'slurm'@'{lookup().control_host}'";""",
        timeout=30,
    )


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

    for f in ("sort_nodes.py",): # copy auxiliary scripts
        dst = Path(lookup().cfg.slurm_bin_dir) / f
        shutil.copyfile(util.scripts_dir / f, dst)
        os.chmod(dst, 0o755)


def setup_controller():
    """Run controller setup"""
    log.info("Setting up controller")
    util.chown_slurm(dirs.scripts / "config.yaml", mode=0o600)
    install_custom_scripts()
    conf.gen_controller_configs(lookup())
    setup_jwt_key()
    setup_munge_key()
    setup_sudoers()
    setup_network_storage()

    run_custom_scripts()

    if not lookup().cfg.cloudsql_secret:
        configure_mysql()

    run("systemctl enable slurmdbd", timeout=30)
    run("systemctl restart slurmdbd", timeout=30)

    # Wait for slurmdbd to come up
    time.sleep(5)

    sacctmgr = f"{slurmdirs.prefix}/bin/sacctmgr -i"
    result = run(
        f"{sacctmgr} add cluster {lookup().cfg.slurm_cluster_name}", timeout=30, check=False
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
    run("systemctl status munge", timeout=30)
    run("systemctl status slurmdbd", timeout=30)
    run("systemctl status slurmctld", timeout=30)
    run("systemctl status slurmrestd", timeout=30)

    sync_slurm()
    run("systemctl enable slurm_load_bq.timer", timeout=30)
    run("systemctl start slurm_load_bq.timer", timeout=30)
    run("systemctl status slurm_load_bq.timer", timeout=30)

    # Add script to perform maintenance
    setup_maintenance_script()

    log.info("Done setting up controller")
    pass


def setup_login():
    """run login node setup"""
    log.info("Setting up login")
    slurmctld_host = f"{lookup().control_host}"
    if lookup().control_addr:
        slurmctld_host = f"{lookup().control_host}({lookup().control_addr})"
    sackd_options = [
        f'--conf-server="{slurmctld_host}:{lookup().control_host_port}"',
    ]
    sysconf = f"""SACKD_OPTIONS='{" ".join(sackd_options)}'"""
    update_system_config("sackd", sysconf)
    install_custom_scripts()

    setup_network_storage()
    setup_sudoers()
    run("systemctl restart munge")
    run("systemctl enable sackd", timeout=30)
    run("systemctl restart sackd", timeout=30)
    run("systemctl enable --now slurmcmd.timer", timeout=30)

    run_custom_scripts()

    log.info("Check status of cluster services")
    run("systemctl status munge", timeout=30)
    run("systemctl status sackd", timeout=30)

    log.info("Done setting up login")


def setup_compute():
    """run compute node setup"""
    log.info("Setting up compute")
    util.chown_slurm(dirs.scripts / "config.yaml", mode=0o600)
    slurmctld_host = f"{lookup().control_host}"
    if lookup().control_addr:
        slurmctld_host = f"{lookup().control_host}({lookup().control_addr})"
    slurmd_options = [
        f'--conf-server="{slurmctld_host}:{lookup().control_host_port}"',
    ]

    try:
        slurmd_feature = util.instance_metadata("attributes/slurmd_feature")
    except Exception:
        # TODO: differentiate between unset and error
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
    run("systemctl restart munge", timeout=30)
    run("systemctl enable slurmd", timeout=30)
    run("systemctl restart slurmd", timeout=30)
    run("systemctl enable --now slurmcmd.timer", timeout=30)

    log.info("Check status of cluster services")
    run("systemctl status munge", timeout=30)
    run("systemctl status slurmd", timeout=30)

    log.info("Done setting up compute")

def setup_cloud_ops() -> None:
    """add deployment info to cloud ops config"""
    cloudOpsStatus = run(
        "systemctl is-active --quiet google-cloud-ops-agent.service", check=False
    ).returncode
    
    if cloudOpsStatus != 0:
        return

    with open("/etc/google-cloud-ops-agent/config.yaml", "r") as f:
        file = yaml.safe_load(f)

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

    with open("/etc/google-cloud-ops-agent/config.yaml", "w") as f:
        yaml.safe_dump(file, f, sort_keys=False)

    run("systemctl restart google-cloud-ops-agent.service", timeout=30)

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
    except subprocess.TimeoutExpired as e:
        log.error(
            f"""TimeoutExpired:
    command={e.cmd}
    timeout={e.timeout}
    stdout:
{e.stdout.strip()}
    stderr:
{e.stderr.strip()}
"""
        )
        log.error("Aborting setup...")
        failed_motd()
    except subprocess.CalledProcessError as e:
        log.error(
            f"""CalledProcessError:
    command={e.cmd}
    returncode={e.returncode}
    stdout:
{e.stdout.strip()}
    stderr:
{e.stderr.strip()}
"""
        )
        log.error("Aborting setup...")
        failed_motd()
    except Exception:
        log.exception("Aborting setup...")
        failed_motd()
