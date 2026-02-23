#!/usr/bin/env python3
# Copyright 2026 Google LLC
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
import shutil
import subprocess
from retry import retry

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

    def start(self):
        try:
            self.workbench.cloud_state = "nm"
            self.workbench.status = "c"
            self.workbench.save()

            self._terraform_init()
            self.workbench.cloud_state = "m"
            self.workbench.status = "i"
            self.workbench.save()
            self._terraform_create()

            # Wait for uri to appear
            self._get_proxy_uri()

            self.workbench.status = "r"
            self.workbench.save()
        except Exception as err: # pylint: disable=broad-except
            logger.error(
                "Encountered error while deploying workbench %d-%s",
                self.workbench.id,
                self.workbench.name,
                exc_info=err
            )


    def terminate(self):
        try:
            self._terraform_destroy()
        except Exception as err: # pylint: disable=broad-except
            logger.error(
                "Encountered error while destroying workbench %d-%s",
                self.workbench.id,
                self.workbench.name,
                exc_info=err
            )

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
        user = self.workbench.trusted_user

        # pylint: disable=line-too-long
        startup_script_vars = f"""
USER=$(curl -s http://metadata.google.internal/computeMetadata/v1/oslogin/users?pagesize=1024 \
            -H 'Metadata-Flavor: Google' | \
        jq '.[][] | \
        select ( .name == "{user.socialaccount_set.first().uid}") | \
        .posixAccounts | \
        .[].username' 2>&- | tr -d '"')
"""

        slurm_config_segment = ""
        try:
            cid = self.workbench.attached_cluster.cloud_id
            slurm_config_segment=f"""\
apt-get install -y munge libmunge-dev


mkdir -p /mnt/clustermunge
mkdir -p /etc/munge
mount slurm-{cid}-controller:/etc/munge /mnt/clustermunge
cp /mnt/clustermunge/munge.key /etc/munge/munge.key
chmod 400 /etc/munge/munge.key
chown munge:munge /etc/munge/munge.key
umount /mnt/clustermunge
rmdir /mnt/clustermunge
systemctl restart munge

useradd --system -u981 -U -m -d /var/lib/slurm -s /bin/bash slurm
echo "N" > /sys/module/nfs/parameters/nfs4_disable_idmapping

tmpdir=$(mktemp -d)
currdir=$PWD
cd $tmpdir
wget https://download.schedmd.com/slurm/slurm-21.08-latest.tar.bz2
tar xf slurm-21.08-latest.tar.bz2
cd slurm-21.08*/


#wget https://download.schedmd.com/slurm/slurm-22.05-latest.tar.bz2
#tar xf slurm-22.05-latest.tar.bz2
#cd slurm-22.05*/

./configure --prefix=/usr/local --sysconfdir=/etc/slurm
make -j $(nproc)
make install
# Throw an error if the slurm install fails
if [ "$?" -ne "0" ]; then
    echo "BRINGUP FAILED"
    exit 1
fi

cd $currdir
rm -r $tmpdir


mkdir -p /etc/slurm
mount slurm-{cid}-controller:/usr/local/etc/slurm /etc/slurm
"""
        except AttributeError:
            pass

        startup_script = self.workbench_dir / "startup_script.sh"
        with startup_script.open("w") as f:
            f.write(
                f"""#!/bin/bash
echo "starting startup script at `date`" | tee -a /tmp/startup.log
echo "Getting username..." | tee -a /tmp/startup.log
{startup_script_vars}

echo "Setting up storage" | tee -a /tmp/startup.log
sudo apt-get -y update && sudo apt-get install -y nfs-common

{slurm_config_segment}

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

EOF

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
        csp_info = f"""
region = "{region}"
zone = "{zone}"
project_name = "{project}"
subnet_name = "{subnet}"
machine_type = "{self.workbench.machine_type}"
boot_disk_type = "{self.workbench.boot_disk_type}"
boot_disk_size_gb = "{self.workbench.boot_disk_capacity}"
trusted_user = "{self.workbench.trusted_user.email}"
image_family = "{self.workbench.image_family}"

owner_id = ["{self.workbench.trusted_user.email}"]
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

    def _terraform_init(self):
        terraform_dir = self.workbench_dir / "terraform"
        extra_env = {
            "GOOGLE_APPLICATION_CREDENTIALS": self._get_credentials_file()
        }

        try:
            self.workbench.cloud_state = "cm"
            self.workbench.save()
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
            self.workbench.status = "e"
            self.workbench.save()
            raise

    def _terraform_create(self):
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
                outputs = json.load(statefp)["outputs"]
                wb_name = "UNKNOWN"
                try:
                    wb_name = outputs["notebook_instance_name"]["value"]
                except KeyError:
                    logger.error(
                        "Failed to parse workbench instance name from TF state"
                    )
                    try:
                        deployment_id = outputs["deployment_id"]["value"]
                        wb_name = f"notebooks-instance-{deployment_id}-0"
                    except KeyError:
                        logger.error(
                            "Failed to parse deployment ID from TF state"
                        )
                # workbench is now being initialized
                self.workbench.internal_name = wb_name
                self.workbench.cloud_state = "m"
                self.workbench.status = "i"

                self.workbench.save()
                # Ansible is now running... Probably 15-30 minutes or so

        except subprocess.CalledProcessError as err:
            # We can error during provisioning, in which case Terraform
            # doesn't tear things down.  Run a `destroy`, just in case
            self.workbench.status = "e"
            self.workbench.cloud_state = "um"
            self.workbench.save()
            logger.error("Terraform apply failed", exc_info=err)
            if err.stdout:
                logger.info("TF stdout:\n%s\n", err.stdout.decode("utf-8"))
            if err.stderr:
                logger.info("TF stderr:\n%s\n", err.stderr.decode("utf-8"))

            logger.error("Attempting to clean up with Terraform destroy")
            self._terraform_destroy()
            raise


    def _terraform_destroy(self):
        terraform_dir = self.workbench_dir / "terraform"
        extra_env = {
            "GOOGLE_APPLICATION_CREDENTIALS": self._get_credentials_file()
        }
        self.workbench.status = "t"
        self.workbench.cloud_state = "dm"
        self.workbench.save()
        try:
            utils.run_terraform(
                terraform_dir / self.cloud_dir, "destroy", extra_env=extra_env
            )
        except subprocess.CalledProcessError as err:
            logger.error("Terraform destroy failed", exc_info=err)
            if err.stdout:
                logger.info("TF stdout:\n%s\n", err.stdout.decode("utf-8"))
            if err.stderr:
                logger.info("TF stderr:\n%s\n", err.stderr.decode("utf-8"))
            logger.error("Resources may still exist - check manually!")
            self.workbench.cloud_state = "um"
            self.workbench.status = "e"
            self.workbench.save()
            raise

        logger.info("Terraform destroy succeeded")
        self.workbench.status = "d"
        self.workbench.cloud_state = "xm"
        self.workbench.save()

    @retry(ValueError, tries=10, delay=30, logger=logger)
    def _get_proxy_uri(self):
        # set terraform dir
        terraform_dir = self.workbench_dir / "terraform"
        extra_env = {
            "GOOGLE_APPLICATION_CREDENTIALS": self._get_credentials_file()
        }

        try:
            utils.run_terraform(
                terraform_dir / self.cloud_dir,
                "apply",
                ["-refresh-only"],
                extra_env=extra_env,
            )
        except subprocess.CalledProcessError as err:
            logger.error("Terraform refresh failed", exc_info=err)
            if err.stdout:
                logger.info("TF stdout:\n%s\n", err.stdout.decode("utf-8"))
            if err.stderr:
                logger.info("TF stderr:\n%s\n", err.stderr.decode("utf-8"))
            raise

        tf_state_file = terraform_dir / self.cloud_dir / "terraform.tfstate"
        with tf_state_file.open("r") as statefp:
            outputs = json.load(statefp)["outputs"]
            try:
                wb_uri = outputs["notebook_proxy_uri"]["value"]
            except KeyError:
                logger.error(
                    "Failed to get workbench uri from TF output"
                )
                raise

        if not wb_uri:
            logger.info("Awaiting workbench uri update (got \"%s\")", wb_uri)
            raise ValueError("Got empty URI")

        logger.info("Got workbench_uri: %s", wb_uri)
        self.workbench.proxy_uri = wb_uri
        self.workbench.save()
