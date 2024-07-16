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
import sys
import stat
import time
from pathlib import Path

import util
from util import (
    lkp,
    cfg,
    dirs,
    slurmdirs,
    run,
    install_custom_scripts,
)

from conf import (
    install_slurm_conf,
    install_slurmdbd_conf,
    gen_cloud_conf,
    gen_cloud_gres_conf,
    gen_topology_conf,
    install_gres_conf,
    install_cgroup_conf,
    install_topology_conf,
    install_jobsubmit_lua,
    login_nodeset,
)
from slurmsync import sync_slurm

from setup_network_storage import (
    setup_network_storage,
    setup_nfs_exports,
)

SETUP_SCRIPT = Path(__file__)
filename = SETUP_SCRIPT.name
LOGFILE = ((cfg.slurm_log_dir if cfg else ".") / SETUP_SCRIPT).with_suffix(".log")
log = logging.getLogger(filename)


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
        "wall -n '*** Slurm {} setup complete ***'".format(lkp.instance_role),
        timeout=30,
    )
    if lkp.instance_role != "controller":
        run(
            """wall -n '
/home on the controller was mounted over the existing /home.
Log back in to ensure your home directory is correct.
'""",
            timeout=30,
        )


def failed_motd():
    """modify motd to signal that setup is failed"""
    wall_msg = f"*** Slurm setup failed! Please view log: {LOGFILE} ***"
    motd_msg = MOTD_HEADER + wall_msg + "\n\n"
    Path("/etc/motd").write_text(motd_msg)
    util.run(f"wall -n '{wall_msg}'", timeout=30)


def run_custom_scripts():
    """run custom scripts based on instance_role"""
    custom_dir = dirs.custom_scripts
    if lkp.instance_role == "controller":
        # controller has all scripts, but only runs controller.d
        custom_dirs = [custom_dir / "controller.d"]
    elif lkp.instance_role == "compute":
        # compute setup with compute.d and nodeset.d
        custom_dirs = [custom_dir / "compute.d", custom_dir / "nodeset.d"]
    elif lkp.instance_role == "login":
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
                timeout = lkp.cfg.get("controller_startup_scripts_timeout", 300)
            elif "/compute.d/" in str(script) or "/nodeset.d/" in str(script):
                timeout = lkp.cfg.get("compute_startup_scripts_timeout", 300)
            elif "/login.d/" in str(script):
                timeout = lkp.cfg.get("login_startup_scripts_timeout", 300)
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
        log.error(f"script {script} encountered an exception")
        log.exception(e)
        raise e


def setup_secondary_disks():
    """Format and mount secondary disk"""
    run(
        "sudo mkfs.ext4 -m 0 -F -E lazy_itable_init=0,lazy_journal_init=0,discard /dev/sdb"
    )
    with open("/etc/fstab", "a") as f:
        f.write(
            "\n/dev/sdb     {0}     ext4    discard,defaults,nofail     0 2".format(
                dirs.secdisk
            )
        )


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
slurm ALL= NOPASSWD: /usr/bin/systemctl restart slurmctld.service
"""
    sudoers_file = Path("/etc/sudoers.d/slurm")
    sudoers_file.write_text(content)
    sudoers_file.chmod(0o0440)


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
        f"""{mysql} "drop user 'slurm'@'{lkp.control_host}'";""",
        timeout=30,
        check=False,
    )
    run(f"""{mysql} "create user 'slurm'@'{lkp.control_host}'";""", timeout=30)
    run(
        f"""{mysql} "grant all on slurm_acct_db.* TO 'slurm'@'{lkp.control_host}'";""",
        timeout=30,
    )


def configure_dirs():
    for p in dirs.values():
        util.mkdirp(p)
    util.chown_slurm(dirs.slurm)
    util.chown_slurm(dirs.scripts)

    for p in slurmdirs.values():
        util.mkdirp(p)
        util.chown_slurm(p)

    etc_slurm = Path("/etc/slurm")
    if etc_slurm.exists() and etc_slurm.is_symlink():
        etc_slurm.unlink()
    etc_slurm.symlink_to(slurmdirs.etc)

    scripts_etc = dirs.scripts / "etc"
    if scripts_etc.exists() and scripts_etc.is_symlink():
        scripts_etc.unlink()
    scripts_etc.symlink_to(slurmdirs.etc)

    scripts_log = dirs.scripts / "log"
    if scripts_log.exists() and scripts_log.is_symlink():
        scripts_log.unlink()
    scripts_log.symlink_to(dirs.log)


def setup_controller(args):
    """Run controller setup"""
    log.info("Setting up controller")
    util.chown_slurm(dirs.scripts / "config.yaml", mode=0o600)
    install_custom_scripts()

    install_slurm_conf(lkp)
    install_slurmdbd_conf(lkp)

    gen_cloud_conf(lkp)
    gen_cloud_gres_conf(lkp)
    gen_topology_conf(lkp)
    install_gres_conf(lkp)
    install_cgroup_conf(lkp)
    install_topology_conf(lkp)
    install_jobsubmit_lua(lkp)

    setup_jwt_key()
    setup_munge_key()
    setup_sudoers()

    if cfg.controller_secondary_disk:
        setup_secondary_disks()
    setup_network_storage(log)

    run_custom_scripts()

    if not cfg.cloudsql_secret:
        configure_mysql()

    run("systemctl enable slurmdbd", timeout=30)
    run("systemctl restart slurmdbd", timeout=30)

    # Wait for slurmdbd to come up
    time.sleep(5)

    sacctmgr = f"{slurmdirs.prefix}/bin/sacctmgr -i"
    result = run(
        f"{sacctmgr} add cluster {cfg.slurm_cluster_name}", timeout=30, check=False
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

    log.info("Done setting up controller")
    pass


def setup_login(args):
    """run login node setup"""
    log.info("Setting up login")
    slurmctld_host = f"{lkp.control_host}"
    if lkp.control_addr:
        slurmctld_host = f"{lkp.control_host}({lkp.control_addr})"
    slurmd_options = [
        f'--conf-server="{slurmctld_host}:{lkp.control_host_port}"',
        f'--conf="Feature={login_nodeset}"',
        "-Z",
    ]
    sysconf = f"""SLURMD_OPTIONS='{" ".join(slurmd_options)}'"""
    update_system_config("slurmd", sysconf)
    install_custom_scripts()

    setup_network_storage(log)
    setup_sudoers()
    run("systemctl restart munge")
    run("systemctl enable slurmd", timeout=30)
    run("systemctl restart slurmd", timeout=30)
    run("systemctl enable --now slurmcmd.timer", timeout=30)

    run_custom_scripts()

    log.info("Check status of cluster services")
    run("systemctl status munge", timeout=30)
    run("systemctl status slurmd", timeout=30)

    log.info("Done setting up login")


def setup_compute(args):
    """run compute node setup"""
    log.info("Setting up compute")
    util.chown_slurm(dirs.scripts / "config.yaml", mode=0o600)
    slurmctld_host = f"{lkp.control_host}"
    if lkp.control_addr:
        slurmctld_host = f"{lkp.control_host}({lkp.control_addr})"
    slurmd_options = [
        f'--conf-server="{slurmctld_host}:{lkp.control_host_port}"',
    ]
    if args.slurmd_feature is not None:
        slurmd_options.append(f'--conf="Feature={args.slurmd_feature}"')
        slurmd_options.append("-Z")
    sysconf = f"""SLURMD_OPTIONS='{" ".join(slurmd_options)}'"""
    update_system_config("slurmd", sysconf)
    install_custom_scripts()

    setup_nss_slurm()
    setup_network_storage(log)

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


def main(args):
    start_motd()
    configure_dirs()

    # call the setup function for the instance type
    setup = dict.get(
        {
            "controller": setup_controller,
            "compute": setup_compute,
            "login": setup_login,
        },
        lkp.instance_role,
        lambda: log.fatal(f"Unknown node role: {lkp.instance_role}"),
    )
    setup(args)

    end_motd()


if __name__ == "__main__":
    util.chown_slurm(LOGFILE, mode=0o600)

    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument(
        "--slurmd-feature",
        dest="slurmd_feature",
        help="Feature for slurmd to register with. Controller ignores this option.",
    )
    args = parser.parse_args()

    util.config_root_logger(filename, logfile=LOGFILE)
    sys.excepthook = util.handle_exception

    lkp = util.Lookup(cfg)  # noqa F811

    try:
        main(args)
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
    except Exception as e:
        log.exception(e)
        log.error("Aborting setup...")
        failed_motd()
