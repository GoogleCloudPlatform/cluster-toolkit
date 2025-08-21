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
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys

import network_storage
import util
import yaml

log = logging.getLogger()

# TODO(b/440182294):
# [X] Fetch real config
# [X] Setup logging file
# [X] Check Filestore
# [ ] Check gsfuse
# [X] Setup & run custom scripts
# [X] Setup prologues / epilogues
# [X] Setup task prologues / epilogues
# [X] Check healthcheck
# [ ] Check with real DNS setup
# [x] Check sinfo
# [ ] Check sacct
# [X] Check srun
# [ ] Check cloud-ops-agent
# [ ] Test resizing of nodeset


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
  """advise in motd that slurm is currently configuring."""
  wall_msg = "*** Slurm is currently being configured in the background. ***"
  motd_msg = MOTD_HEADER + wall_msg + "\n\n"
  Path("/etc/motd").write_text(motd_msg)
  util.run(f"wall -n '{wall_msg}'", timeout=30)


def end_motd():
  """modify motd to signal that setup is complete."""
  Path("/etc/motd").write_text(MOTD_HEADER)
  util.run("wall -n '*** Slurm setup complete ***'", timeout=30)


def failed_motd():
  """modify motd to signal that setup is failed."""
  wall_msg = (
      "*** Slurm setup failed! Please view log: /var/log/slurm/setup.log ***"
  )
  motd_msg = MOTD_HEADER + wall_msg + "\n\n"
  Path("/etc/motd").write_text(motd_msg)
  util.run(f"wall -n '{wall_msg}'", timeout=30)


def run_custom_scripts(cfg: util.Config):
  """run custom scripts based on instance_role."""
  suffix = "nodeset.d" if util.instance_role() == "compute" else "login.d"
  sdir = util.CUSTOM_SCRIPTS_DIR / suffix

  timeout = cfg.startup_script_timeout
  for script in [s for s in sdir.rglob("*") if s.is_file()]:
    log.info("running script %s with timeout=%ds", script.name, timeout)
    util.run(str(script), timeout=timeout, check=True, shell=True)


def update_system_config(file: str, content: str) -> None:
  """Add system defaults options for service files."""
  sysconfig = Path("/etc/sysconfig")
  default = Path("/etc/default")

  if sysconfig.exists():
    conf_dir = sysconfig
  elif default.exists():
    conf_dir = default
  else:
    raise Exception("Cannot determine system configuration directory.")
  Path(conf_dir, file).write_text(content)


# TODO(b/440193886): Should be done as part of image building
def setup_nss_slurm():
  """install and configure nss_slurm."""
  # setup nss_slurm
  Path("/var/spool/slurmd").mkdir(parents=True, exist_ok=True)
  util.run(
      "ln -s /usr/local/lib/libnss_slurm.so.2 /usr/lib64/libnss_slurm.so.2",
      check=False,
  )
  util.run(
      r"sed -i 's/\(^\(passwd\|group\):\s\+\)/\1slurm /g' /etc/nsswitch.conf"
  )


# TODO(b/440193755): Should be done as part of image building
def setup_sudoers():
  content = """
# Allow SlurmUser to manage the slurm daemons
slurm ALL= NOPASSWD: /usr/bin/systemctl restart slurmd.service
slurm ALL= NOPASSWD: /usr/bin/systemctl restart sackd.service
"""
  sudoers_file = Path("/etc/sudoers.d/slurm")
  sudoers_file.write_text(content)
  sudoers_file.chmod(0o0440)


# TODO(b/440193118): Should be done as part of image building
def configure_dirs():
  # copy auxiliary scripts
  for dst_folder, src_file in (
      (util.CUSTOM_SCRIPTS_DIR / "task_prolog.d", Path("tools/task-prolog")),
      (util.CUSTOM_SCRIPTS_DIR / "task_epilog.d", Path("tools/task-epilog")),
  ):
    dst = Path(dst_folder) / src_file.name
    dst.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(util.SCRIPTS_DIR / src_file, dst)
    os.chmod(dst, 0o755)


def setup_login(cfg: util.Config) -> None:
  """run login node setup."""
  log.info("Setting up login")

  sackd_options = [f'--conf-server="{util.controller_host()}:6820-6830"']

  sysconf = f"""SACKD_OPTIONS='{" ".join(sackd_options)}'"""
  update_system_config("sackd", sysconf)
  util.install_custom_scripts()

  network_storage.setup_network_storage(cfg)
  network_storage.slurm_key_mount_handler()
  setup_sudoers()

  util.run("systemctl enable sackd", timeout=30)
  util.run("systemctl restart sackd", timeout=30)
  util.run("systemctl enable --now slurmcmd.timer", timeout=30)

  run_custom_scripts(cfg)

  log.info("Check status of cluster services")
  util.run("systemctl status sackd", timeout=30)

  log.info("Done setting up login")


def setup_compute(cfg: util.Config):
  """run compute node setup."""
  log.info("Setting up compute")

  slurmd_options = [
      f'--conf-server="{util.controller_host()}:6820-6830"',
  ]

  try:
    slurmd_feature = util.instance_metadata(
        "attributes/slurmd_feature", silent=True
    )
  except util.MetadataNotFoundError:
    slurmd_feature = None

  if slurmd_feature is not None:
    slurmd_options.append(f'--conf="Feature={slurmd_feature}"')
    slurmd_options.append("-Z")

  sysconf = f"""SLURMD_OPTIONS='{" ".join(slurmd_options)}'"""
  update_system_config("slurmd", sysconf)
  util.install_custom_scripts()

  setup_nss_slurm()
  network_storage.setup_network_storage(cfg)
  network_storage.slurm_key_mount_handler()

  run_custom_scripts(cfg)

  setup_sudoers()

  util.run("systemctl enable slurmd", timeout=30)
  util.run("systemctl restart slurmd", timeout=30)
  util.run("systemctl enable --now slurmcmd.timer", timeout=30)

  log.info("Check status of cluster services")
  util.run("systemctl status slurmd", timeout=30)

  log.info("Done setting up compute")


def setup_cloud_ops() -> None:
  """Add health checks, deployment info, and updated setup path to cloud ops config."""
  conf_file = Path("/etc/google-cloud-ops-agent/config.yaml")
  if not conf_file.exists():
    log.error("google-cloud-ops-agent is not installed. Skipping setup.")
    return

  with open("/etc/google-cloud-ops-agent/config.yaml", "r") as f:
    file = yaml.safe_load(f)

  # Update setup receiver path
  # TODO(b/440194904): Should be done as part of image building
  file["logging"]["receivers"]["setup"]["include_paths"] = [
      "/var/log/slurm/setup.log"
  ]

  # Add chs_health_check receiver
  # TODO(b/440194904): Should be done as part of image building
  file["logging"]["receivers"]["chs_health_check"] = {
      "type": "files",
      "include_paths": ["/var/log/slurm/chs_health_check.log"],
      "record_log_file_path": True,
  }

  cluster_info = {
      "type": "modify_fields",
      "fields": {
          'labels."cluster_name"': {"static_value": f"{util.cluster_name()}"},
          'labels."hostname"': {"static_value": f"{socket.gethostname()}"},
      },
  }

  file["logging"]["processors"]["add_cluster_info"] = cluster_info
  file["logging"]["service"]["pipelines"]["slurmlog_pipeline"][
      "processors"
  ].append("add_cluster_info")
  file["logging"]["service"]["pipelines"]["slurmlog2_pipeline"][
      "processors"
  ].append("add_cluster_info")

  # Add chs_health_check to slurmlog2_pipeline
  # TODO(b/440194904): Should be done as part of image building
  file["logging"]["service"]["pipelines"]["slurmlog2_pipeline"][
      "receivers"
  ].append("chs_health_check")

  with open(conf_file, "w") as f:
    yaml.safe_dump(file, f, sort_keys=False)

  retries = 2
  for _ in range(retries):
    try:
      util.run("systemctl restart google-cloud-ops-agent.service", timeout=120)
      break
    except subprocess.TimeoutExpired:
      log.error("google-cloud-ops-agent.service did not restart within 120s.")
      result = util.run(
          "cat /var/log/google-cloud-ops-agent/subagents/logging-module.log",
          timeout=120,
          shell=True,
      )
      if result.stdout:
        log.error("Logs for google-cloud-ops-agent:\n%s", result.stdout)
      raise


def main():
  start_motd()

  assert util.instance_role() in (
      "login",
      "compute",
  ), f"Unknown instance role: {util.instance_role()}"

  log.info("Starting setup, fetching config")
  _ = util.fetch_config()
  setup_cloud_ops()
  configure_dirs()

  if util.instance_role() == "login":
    setup_login(util.config())
  else:
    setup_compute(util.config())

  end_motd()


if __name__ == "__main__":
  try:
    util.init_log("setup")
    main()
  except Exception:
    log.exception("Aborting setup...")
    failed_motd()
    sys.exit(1)
