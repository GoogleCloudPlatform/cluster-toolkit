#!/usr/bin/env python3
# Copyright 2022 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Workbench configuration and provisioning"""

import logging
import json
import os
import shutil
import subprocess
from datetime import datetime, timedelta

from . import utils

logger = logging.getLogger(__name__)


class WorkbenchInfo:
    """Workbench configuration and management"""

    def __init__(self, workbench):
        self.config = utils.load_config()

        self.workbench = workbench
        self.workbench_dir = (
            self.config["baseDir"]
            / "workbenches"
            / f"workbench_{self.workbench.id}"
        )

        self.cloud_dir = "google"

    def create_workbench_dir(self, credentials):
        self.workbench_dir.mkdir(parents=True)

        self.set_credentials(credentials)
        self.copy_terraform()
        self.copy_startup_script()
        self.prepare_terraform_vars()

    def _get_credentials_file(self):
        return self.workbench_dir / "cloud_credentials"

    def set_credentials(self, creds=None):
        credfile = self._get_credentials_file()
        if not creds:
            # pull from DB
            creds = self.workbench.cloud_credential.detail
        with credfile.open("w") as fp:
            fp.write(creds)

    def copy_terraform(self):
        terraform_dir = self.workbench_dir / "terraform"
        shutil.copytree(
            self.config["baseDir"]
            / "infrastructure_files"
            / "workbench_tf"
            / self.cloud_dir,
            terraform_dir / self.cloud_dir,
        )
        return terraform_dir

    def copy_startup_script(self):
        user = self.workbench.trusted_users

        # pylint: disable=line-too-long
        startup_script_vars = f"""
USER=$(curl -s http://metadata.google.internal/computeMetadata/v1/oslogin/users?pagesize=1024 \
            -H 'Metadata-Flavor: Google' | \
        jq '.[][] | \
        select ( .name == "{user.socialaccount_set.first().uid}") | \
        .posixAccounts | \
        .[].username' 2>&- | tr -d '"')
"""

        startup_script = self.workbench_dir / "startup_script.sh"
        with startup_script.open("w") as f:
            f.write(
                f"""#!/bin/bash
echo "starting starup script at `date`" | tee -a /tmp/startup.log
echo "Getting username..." | tee -a /tmp/startup.log
{startup_script_vars}

echo "Setting up storage" | tee -a /tmp/startup.log
sudo apt-get -y update && sudo apt-get install -y nfs-common

mkdir /tmp/jupyterhome
chown $USER:$USER /tmp/jupyterhome

mkdir /home/$USER
chown $USER:$USER /home/$USER

cp /home/jupyter/.jupyter /tmp/jupyterhome/.jupyter -R
chown $USER:$USER /tmp/jupyterhome/.jupyter -R

cat << EOF > /tmp/jupyterhome/DATA_LOSS_WARNING.txt
DATA LOSS WARNING:

The data on this workbench instance is not automatically saved unless it is
saved in a shared filesystem that has been mounted.

All mounted shared filesystems are listed below. If none are listed then
all data on this instance will be deleted.

MOUNTED FILESYSTEMS:

"""
            )
            for mp in self.workbench.mount_points.order_by("mount_order"):
                if (
                    self.workbench.id == mp.workbench.id
                    and mp.export.filesystem.hostname_or_ip
                ):
                    f.write("mkdir -p " + mp.mount_path + "\n")
                    f.write(
                        "mkdir -p /tmp/jupyterhome`dirname "
                        + mp.mount_path
                        + "`\n"
                    )
                    f.write(
                        "mount "
                        + mp.export.filesystem.hostname_or_ip
                        + ":"
                        + mp.export.export_name
                        + " "
                        + mp.mount_path
                        + "\n"
                    )
                    f.write("chmod 777 " + mp.mount_path + "\n")
                    f.write(
                        "ln -s "
                        + mp.mount_path
                        + " /tmp/jupyterhome`dirname "
                        + mp.mount_path
                        + "` \n"
                    )
                    f.write(
                        'echo "'
                        + mp.export.filesystem.hostname_or_ip
                        + ":"
                        + mp.export.export_name
                        + " is mounted at "
                        + mp.mount_path
                        + '" >> /tmp/jupyterhome/DATA_LOSS_WARNING.txt\n'
                    )

            logger.debug("Writing workbench startup script")
            with open(
                self.config["baseDir"]
                / "infrastructure_files"
                / "gcs_bucket"
                / "workbench"
                / "startup_script_template.sh",
                encoding="utf-8",
            ) as infile:
                for line in infile:
                    print(line)
                    f.write(line)

                f.write("\n")

    def prepare_terraform_vars(self):
        region = self.workbench.cloud_region
        zone = self.workbench.cloud_zone
        subnet = self.workbench.subnet.cloud_id

        # Cloud-specific Terraform changes
        project = json.loads(self.workbench.cloud_credential.detail)[
            "project_id"
        ]
        user = self.workbench.trusted_users
        trusted_user_tfvalue = '"user:' + user.email + '"'

        csp_info = f"""
region = "{region}"
zone = "{zone}"
project_name = "{project}"
subnet_name = "{subnet}"
machine_type = "{self.workbench.machine_type}"
boot_disk_type = "{self.workbench.boot_disk_type}"
boot_disk_size_gb = "{self.workbench.boot_disk_capacity}"
trusted_users = [{trusted_user_tfvalue}]
image_family = "{self.workbench.image_family}"

owner_id = ["{user.email}"]
wb_startup_script_name   = "workbench/workbench_{self.workbench.id}_startup_script"
wb_startup_script_bucket = "{self.config["server"]["gcs_bucket"]}"
"""
        tfvars = (
            self.workbench_dir
            / "terraform"
            / self.cloud_dir
            / "terraform.tfvars"
        )
        with tfvars.open("w") as f:
            f.write(
                f"""
{csp_info}
"""
            )

    def get_workbench_access_key(self):
        return self.workbench.get_access_key()

    def initialize_terraform(self):
        terraform_dir = self.workbench_dir / "terraform"
        extra_env = {
            "GOOGLE_APPLICATION_CREDENTIALS": self._get_credentials_file()
        }

        try:
            utils.run_terraform(terraform_dir / self.cloud_dir, "init")
            utils.run_terraform(
                terraform_dir / self.cloud_dir, "validate", extra_env=extra_env
            )
            utils.run_terraform(
                terraform_dir / self.cloud_dir, "plan", extra_env=extra_env
            )
        except subprocess.CalledProcessError as cpe:
            if cpe.stdout:
                print(cpe.stdout.decode("utf-8"))
            if cpe.stderr:
                print(cpe.stderr.decode("utf-8"))
            raise

    def run_terraform(self):
        terraform_dir = self.workbench_dir / "terraform"
        extra_env = {
            "GOOGLE_APPLICATION_CREDENTIALS": self._get_credentials_file()
        }
        try:
            utils.run_terraform(
                terraform_dir / self.cloud_dir, "apply", extra_env=extra_env
            )
            # Look for Management Public IP in terraform.tfstate
            tf_state_file = terraform_dir / self.cloud_dir / "terraform.tfstate"
            with tf_state_file.open("r") as statefp:
                state = json.load(statefp)
                workbench_name = state["outputs"]["workbench_id"]["value"]
                # workbench is now being initialized
                self.workbench.internal_name = workbench_name
                self.workbench.cloud_state = "m"
                self.workbench.status = "i"

                self.workbench.save()
                # Ansible is now running... Probably 15-30 minutes or so

        except subprocess.CalledProcessError as err:
            # We can error during provisioning, in which case Terraform
            # doesn't tear things down.  Run a `destroy`, just in case
            logger.error("Terraform apply failed", exc_info=err)
            if err.stdout:
                logger.info("TF stdout:\n%s\n", err.stdout.decode("utf-8"))
            if err.stderr:
                logger.info("TF stderr:\n%s\n", err.stderr.decode("utf-8"))
            try:
                logger.error("Attempting to clean up with Terraform destroy")
                utils.run_terraform(terraform_dir / self.cloud_dir, "destroy")
            except subprocess.CalledProcessError as err2:
                logger.error("Terraform destroy failed", exc_info=err2)
                if err2.stdout:
                    logger.info("TF stdout:\n%s\n", err2.stdout.decode("utf-8"))
                if err2.stderr:
                    logger.info("TF stderr:\n%s\n", err2.stderr.decode("utf-8"))
                logger.error("Resources may still exist - check manually!")
                raise
            else:
                logger.error("Terraform destroy succeeded")
            raise

    def get_workbench_proxy_uri(self):
        # set terraform dir
        terraform_dir = self.workbench_dir / "terraform"
        extra_env = {
            "GOOGLE_APPLICATION_CREDENTIALS": self._get_credentials_file()
        }

        try:
            tf_state_file = terraform_dir / self.cloud_dir / "terraform.tfstate"
            if os.path.exists(tf_state_file):
                file_time = datetime.utcfromtimestamp(
                    os.path.getmtime(tf_state_file)
                )
                check_time = datetime.utcnow() - timedelta(seconds=60)

                if file_time < check_time:
                    utils.run_terraform(
                        terraform_dir / self.cloud_dir,
                        "apply",
                        ["-refresh-only"],
                        extra_env=extra_env,
                    )

                    with tf_state_file.open("r") as statefp:
                        state = json.load(statefp)
                        try:
                            self.workbench.proxy_uri = state["resources"][3][
                                "instances"
                            ][0]["attributes"]["proxy_uri"]
                            if (
                                state["resources"][3]["instances"][0][
                                    "attributes"
                                ]["state"]
                                == "ACTIVE"
                            ):
                                self.workbench.status = "r"

                        except Exception as err:  # pylint: disable=broad-except
                            logger.error(
                                "Failed to read terraform state file: %s", err
                            )
                            raise

                        self.workbench.save()

        except subprocess.CalledProcessError as err:
            logger.error("Terraform refresh failed", exc_info=err)
            if err.stdout:
                logger.info("TF stdout:\n%s\n", err.stdout.decode("utf-8"))
            if err.stderr:
                logger.info("TF stderr:\n%s\n", err.stderr.decode("utf-8"))
            raise
