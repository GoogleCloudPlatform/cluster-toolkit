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


# To create a workbench, we need:
# 1) Know which Cloud Provider & region/zone/project
# 2) Know authentication credentials
# 3) Know an "ID Number" or name - for directory to store state info

# 1 - Supplied via commandline
# 2 - Supplied via... Env vars / commandline?
# 3 - Supplied via commandline

import argparse
import os, shutil, sys
import subprocess
import json
import os.path as path
import requests
from datetime import datetime
from datetime import timedelta

from . import utils
from ..models import WorkbenchMountPoint
class WorkbenchInfo:

    def __init__(self, workbench):
        self.config = utils.load_config()

        self.workbench = workbench
        self.workbench_dir = self.config["baseDir"] / 'workbenches' / f'workbench_{self.workbench.id}'

        self.cloud_dir = "google"

    def create_workbench_dir(self, credentials):
        self.workbench_dir.mkdir(parents=True)

        self.set_credentials(credentials)
        self.copy_terraform()
        self.copy_startup_script()
        self.prepare_terraform_vars()
    
    def _get_credentials_file(self):
        return self.workbench_dir / 'cloud_credentials'

    def get_credentials_file(self):
        return self.workbench_dir / 'cloud_credentials'

    def set_credentials(self, creds=None):
        credfile = self.get_credentials_file()
        if not creds:
            # pull from DB
            creds = self.workbench.cloud_credential.detail
        with credfile.open('w') as fp:
                fp.write(creds)

    def copy_terraform(self):
        tfDir = self.workbench_dir / 'terraform'
        shutil.copytree(self.config["baseDir"] / 'infrastructure_files' / 'workbench_tf' / self.cloud_dir, tfDir / self.cloud_dir )
        return tfDir

    def copy_startup_script(self):
        user = self.workbench.trusted_users

        startup_script_vars = f"""
USER=`curl -s http://metadata.google.internal/computeMetadata/v1/oslogin/users?pagesize=1024 -H 'Metadata-Flavor: Google' | jq '.[][] | select ( .name == "{user.socialaccount_set.first().uid}") | .posixAccounts | .[].username' 2>&- | tr -d '"'`
"""
   
        startup_script = self.workbench_dir / 'startup_script.sh'
        with startup_script.open('w') as f:
            f.write(f"""#!/bin/bash
{startup_script_vars}
""")
            with open(self.config["baseDir"] / 'infrastructure_files' / 'gcs_bucket' / 'workbench' / 'startup_script_template.sh') as infile:
                for line in infile:
                    f.write(line)
                
                f.write("\n")

                for mp in WorkbenchMountPoint.objects.all():
                    if self.workbench.id == mp.workbench.id and mp.export.filesystem.hostname_or_ip:
                        f.write("mkdir " + mp.mount_path + "\n")
                        f.write("mount " + mp.export.filesystem.hostname_or_ip + ":" + mp.export.export_name + " " + mp.mount_path +"\n")
                        f.write("ln -s " + mp.mount_path + " /home/$USER/mount_points \n")


    def prepare_terraform_vars(self):
        region = self.workbench.cloud_region
        zone = self.workbench.cloud_zone
        vpc = self.workbench.subnet.vpc.cloud_id
        subnet = self.workbench.subnet.cloud_id

        # Cloud-specific Terraform changes
        project = json.loads(self.workbench.cloud_credential.detail)["project_id"]
        user = self.workbench.trusted_users
        trusted_user_tfvalue = "\"user:" + user.email + "\""

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
#        pkeys_str = b"\n".join(self._get_ssh_keys()).decode('utf-8')
        tfvars = self.workbench_dir / 'terraform' / self.cloud_dir / 'terraform.tfvars'
        with tfvars.open('w') as f:
            f.write(f"""
{csp_info}
""")


    def get_workbench_access_key(self):
        return self.workbench.get_access_key()

    def initialize_terraform(self):
        tfDir = self.workbench_dir / 'terraform'
        extraEnv = {'GOOGLE_APPLICATION_CREDENTIALS': self._get_credentials_file()}
        #            utils.run_terraform(tfDir, "validate", extraEnv=extraEnv)

        try:
            utils.run_terraform(tfDir / self.cloud_dir, "init")
            utils.run_terraform(tfDir / self.cloud_dir, "validate", extraEnv=extraEnv)
            utils.run_terraform(tfDir / self.cloud_dir, "plan", extraEnv=extraEnv)
        except subprocess.CalledProcessError as cpe:
            if cpe.stdout:
                print(cpe.stdout.decode('utf-8'))
            if cpe.stderr:
                print(cpe.stderr.decode('utf-8'))
            raise


    def run_terraform(self):
        tfDir = self.workbench_dir / 'terraform'
        extraEnv = {'GOOGLE_APPLICATION_CREDENTIALS': self._get_credentials_file()}
        try:
            (log_out, log_err) = utils.run_terraform(tfDir / self.cloud_dir, "apply", extraEnv=extraEnv)
            # Look for Management Public IP in terraform.tfstate
            stateFile = tfDir / self.cloud_dir / 'terraform.tfstate'
            with stateFile.open('r') as statefp:
                state = json.load(statefp)
                workbench_name = state["outputs"]["workbench_id"]["value"]
                print(f"Created workbench {workbench_name}, url not available yet")
                # workbench is now being initialized
                self.workbench.internal_name = workbench_name
                self.workbench.cloud_state = 'm'
                self.workbench.status = 'i'

                self.workbench.save()
                # Ansible is now running... Probably 15-30 minutes or so


        except subprocess.CalledProcessError as cpe:
            # We can error during provisioning, in which case Terraform
            # doesn't tear things down.  Run a `desotry`, just in case
            try:
                utils.run_terraform(tfDir / self.cloud_dir, 'destroy')
            except subprocess.CalledProcessError as cpe2:
                pass
            if cpe.stdout:
                print(cpe.stdout.decode('utf-8'))
            if cpe.stderr:
                print(cpe.stderr.decode('utf-8'))
            raise

    def get_workbench_proxy_uri(self):
        # set terraform dir
        tfDir = self.workbench_dir / 'terraform'
        extraEnv = {'GOOGLE_APPLICATION_CREDENTIALS': self._get_credentials_file()}

        try:
            stateFile = tfDir / self.cloud_dir / 'terraform.tfstate'
            if os.path.exists(stateFile):
                file_time = datetime.utcfromtimestamp(os.path.getmtime(stateFile))
                check_time = datetime.utcnow() - timedelta(seconds=60)
                
                if file_time < check_time:
                    (log_out, log_err) = utils.run_terraform(tfDir / self.cloud_dir, "apply", ["-refresh-only"], extraEnv=extraEnv)
                    
                    with stateFile.open('r') as statefp:
                        state = json.load(statefp)
                        try:
                            self.workbench.proxy_uri = state["resources"][3]["instances"][0]["attributes"]["proxy_uri"]
                            if state["resources"][3]["instances"][0]["attributes"]["state"] == "ACTIVE":
                                self.workbench.status = 'r'
                        except Exception:
                            pass

                        self.workbench.save()

        
        except subprocess.CalledProcessError as cpe:
            # We can error during provisioning, in which case Terraform
            # doesn't tear things down.  Run a `desotry`, just in case
            if cpe.stdout:
                print(cpe.stdout.decode('utf-8'))
            if cpe.stderr:
                print(cpe.stderr.decode('utf-8'))
            raise
