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

import logging
from pathlib import Path
import time

import shutil
import os
import yaml
import subprocess
import socket

import util
import network_storage

# !!!  logging file
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


def start_motd():
    """advise in motd that slurm is currently configuring"""
    wall_msg = "*** Slurm is currently being configured in the background. ***"
    motd_msg = MOTD_HEADER + wall_msg + "\n\n"
    Path("/etc/motd").write_text(motd_msg)
    util.run(f"wall -n '{wall_msg}'", timeout=30)


def failed_motd():
    """modify motd to signal that setup is failed"""
    wall_msg = f"*** Slurm setup failed! Please view log: /var/log/slurm/setup.log ***"
    motd_msg = MOTD_HEADER + wall_msg + "\n\n"
    Path("/etc/motd").write_text(motd_msg)
    util.run(f"wall -n '{wall_msg}'", timeout=30)

def end_motd():
    """modify motd to signal that setup is complete"""
    Path("/etc/motd").write_text(MOTD_HEADER)
    util.run("wall -n '*** Slurm setup complete ***'", timeout=30)
    

def update_system_config(file: str, content: str) -> None:
    """Add system defaults options for service files"""
    sysconfig = Path("/etc/sysconfig")
    default = Path("/etc/default")

    if sysconfig.exists():
        conf_dir = sysconfig
    elif default.exists():
        conf_dir = default
    else:
        raise Exception("Cannot determine system configuration directory.")
    Path(conf_dir, file).write_text(content)


# TODO(b/XXXXX): Should be done as part of image building
def setup_nss_slurm():
    """install and configure nss_slurm"""
    # setup nss_slurm
    Path("/var/spool/slurmd").mkdir(parents=True, exist_ok=True)
    util.run("ln -s /usr/local/lib/libnss_slurm.so.2 /usr/lib64/libnss_slurm.so.2", check=False)
    util.run(r"sed -i 's/\(^\(passwd\|group\):\s\+\)/\1slurm /g' /etc/nsswitch.conf")

# TODO(b/XXXXX): Should be done as part of image building
def setup_sudoers():
    content = """
# Allow SlurmUser to manage the slurm daemons
slurm ALL= NOPASSWD: /usr/bin/systemctl restart slurmd.service
slurm ALL= NOPASSWD: /usr/bin/systemctl restart sackd.service
"""
    sudoers_file = Path("/etc/sudoers.d/slurm")
    sudoers_file.write_text(content)
    sudoers_file.chmod(0o0440)


def setup_login():
    """run login node setup"""
    log.info("Setting up login")

    sackd_options = [f'--conf-server="{util.controller_host()}:6820-6830"']
    
    sysconf = f"""SACKD_OPTIONS='{" ".join(sackd_options)}'"""
    update_system_config("sackd", sysconf)
    #util.install_custom_scripts()

    network_storage.setup_network_storage(util.config())
    network_storage.slurm_key_mount_handler()
    setup_sudoers()
    
    util.run("systemctl enable sackd", timeout=30)
    util.run("systemctl restart sackd", timeout=30)
    util.run("systemctl enable --now slurmcmd.timer", timeout=30)

    # !!! run_custom_scripts()

    log.info("Check status of cluster services")
    util.run("systemctl status sackd", timeout=30)

    log.info("Done setting up login")

def setup_compute():
    """run compute node setup"""
    log.info("Setting up compute")
    
    slurmd_options = [f'--conf-server="{util.controller_host()}:6820-6830"',]

    try:
        slurmd_feature = util.instance_metadata("attributes/slurmd_feature", silent=True)
    except util.MetadataNotFoundError:
        slurmd_feature = None

    if slurmd_feature is not None:
        slurmd_options.append(f'--conf="Feature={slurmd_feature}"')
        slurmd_options.append("-Z")

    sysconf = f"""SLURMD_OPTIONS='{" ".join(slurmd_options)}'"""
    update_system_config("slurmd", sysconf)
    #util.install_custom_scripts()

    setup_nss_slurm()
    network_storage.setup_network_storage(util.config())
    network_storage.slurm_key_mount_handler()

    # !!! run_custom_scripts()

    setup_sudoers()
    
    util.run("systemctl enable slurmd", timeout=30)
    util.run("systemctl restart slurmd", timeout=30)
    util.run("systemctl enable --now slurmcmd.timer", timeout=30)

    log.info("Check status of cluster services")
    util.run("systemctl status slurmd", timeout=30)

    log.info("Done setting up compute")


# !!! dirs = NSDict(
#     home = Path("/home"),
#     slurm = Path("/slurm"),
#     scripts = scripts_dir,
#     custom_scripts = Path("/slurm/custom_scripts"),
#     log = Path("/var/log/slurm"),
# )

# slurmdirs = NSDict(
#     prefix = Path("/usr/local"),
#     etc = Path("/usr/local/etc/slurm"),
#     key_distribution = Path("/slurm/key_distribution"),
# )


def configure_dirs():
    pass # !!!
    # # TODO(b/XXXXX): Should be done as part of image building
    # for p in dirs.values():
    #     util.mkdirp(p)

    # # TODO(b/XXXXX): Should be done as part of image building
    # for p in (dirs.slurm, dirs.scripts, dirs.custom_scripts):
    #     util.chown_slurm(p)

    # # TODO(b/XXXXX): Should be done as part of image building
    # for p in slurmdirs.values():
    #     util.mkdirp(p)
    #     util.chown_slurm(p)

    # # TODO(b/XXXXX): Should be done as part of image building
    # for sl, tgt in ( # create symlinks
    #     (Path("/etc/slurm"), slurmdirs.etc),
    #     (dirs.scripts / "etc", slurmdirs.etc),
    #     (dirs.scripts / "log", dirs.log),
    # ):
    #     if sl.exists() and sl.is_symlink():
    #         sl.unlink()
    #     sl.symlink_to(tgt)

    # # copy auxiliary scripts
    # # TODO(b/XXXXX): Should be done as part of image building
    # for dst_folder, src_file in ((dirs.custom_scripts / "task_prolog.d",
    #                               Path("tools/task-prolog")),
    #                              (dirs.custom_scripts / "task_epilog.d",
    #                               Path("tools/task-epilog"))):
    #     dst = Path(dst_folder) / src_file.name
    #     util.mkdirp(dst.parent)
    #     shutil.copyfile(util.scripts_dir / src_file, dst)
    #     os.chmod(dst, 0o755)


def setup_cloud_ops() -> None:
    """Add health checks, deployment info, and updated setup path to cloud ops config."""
    cloud_ops_status = util.run(
        "systemctl is-active --quiet google-cloud-ops-agent.service", check=False
    ).returncode

    if cloud_ops_status != 0:
        return

    with open("/etc/google-cloud-ops-agent/config.yaml", "r") as f:
        file = yaml.safe_load(f)

    # Update setup receiver path
    # TODO(b/XXXXX): Should be done as part of image building
    file["logging"]["receivers"]["setup"]["include_paths"] = ["/var/log/slurm/setup.log"]

    # Add chs_health_check receiver
    # TODO(b/XXXXX): Should be done as part of image building
    file["logging"]["receivers"]["chs_health_check"] = {
        "type": "files",
        "include_paths": ["/var/log/slurm/chs_health_check.log"],
        "record_log_file_path": True,
    }

    cluster_info = {
        'type':'modify_fields',
        'fields': {
            'labels."cluster_name"':{
                'static_value':f"{util.cluster_name()}"
            },
            'labels."hostname"':{
                'static_value': f"{socket.gethostname()}"
            }
        }
    }

    file["logging"]["processors"]["add_cluster_info"] = cluster_info
    file["logging"]["service"]["pipelines"]["slurmlog_pipeline"]["processors"].append("add_cluster_info")
    file["logging"]["service"]["pipelines"]["slurmlog2_pipeline"]["processors"].append("add_cluster_info")

    # Add chs_health_check to slurmlog2_pipeline
    # TODO(b/XXXXX): Should be done as part of image building
    file["logging"]["service"]["pipelines"]["slurmlog2_pipeline"]["receivers"].append(
        "chs_health_check"
    )

    with open("/etc/google-cloud-ops-agent/config.yaml", "w") as f:
        yaml.safe_dump(file, f, sort_keys=False)

    retries = 2
    for _ in range(retries):
        try:
            util.run("systemctl restart google-cloud-ops-agent.service", timeout=120)
            break
        except subprocess.TimeoutExpired:
            log.error("google-cloud-ops-agent.service did not restart within 120s.")
            result=util.run("cat /var/log/google-cloud-ops-agent/subagents/logging-module.log", timeout=120, shell=True)
            if result.stdout:
                log.error(f"Logs for google-cloud-ops-agent (logging-module.log file):\n{result.stdout}")
            raise


def run_custom_scripts():
    """run custom scripts based on instance_role"""
    custom_dir = dirs.custom_scripts
    
    elif lookup().instance_role == "compute":
        # compute setup with nodeset.d
        custom_dirs = [custom_dir / "nodeset.d"]
    elif lookup().is_login_node:
        # login setup with only login.d
        custom_dirs = [custom_dir / "login.d"]
    else:
        # Unknown role: run nothing
        custom_dirs = []

    timeout = cfg.startup_script_timeout
    custom_scripts = [
        p
        for d in custom_dirs
        for p in d.rglob("*")
        if p.is_file() and not p.name.endswith(".disabled")
    ]
    print_scripts = ",".join(str(s.relative_to(custom_dir)) for s in custom_scripts)
    log.debug(f"custom scripts to run: {custom_dir}/({print_scripts})")

    
    for script in custom_scripts:
        log.info(f"running script {script.name} with timeout={timeout}")
        util.run(str(script), timeout=timeout, check=True, shell=True)
    

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

    if util.instance_role() == "login":
        setup_login()
    elif util.instance_role() == "compute":
        setup_compute()
    else:
        raise Exception(f"Unknown instance role: {util.instance_role()}")
    
    end_motd()

if __name__ == "__main__":
    try:
        main()
    except Exception:
        log.exception("Aborting setup...")
        failed_motd()
        # !!! sys exit 1?
        