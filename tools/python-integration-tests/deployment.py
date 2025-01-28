# Copyright 2024 "Google LLC"
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

import json
import shutil
import os
import subprocess
import yaml

class Deployment:
    def __init__(self, blueprint: str):
        self.blueprint_yaml = blueprint
        self.state_bucket = "daily-tests-tf-state"
        self.project_id = None
        self.workspace = None
        self.instance_name = None
        self.username = None
        self.deployment_name = None
        self.zone = None

    def run_command(self, cmd: str, err_msg: str = None) -> subprocess.CompletedProcess:
        res = subprocess.run(cmd, shell=True, universal_newlines=True, check=True,
                         stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        return res

    def parse_blueprint(self, file_path: str):
        with open(file_path, 'r') as file:
            content = yaml.safe_load(file)
        self.deployment_name = content["vars"]["deployment_name"]
        self.zone = content["vars"]["zone"]

    def get_posixAccount_info(self):
        # Extract the username from posixAccounts
        result = self.run_command(f"gcloud compute os-login describe-profile --format=json").stdout
        posixAccounts = json.loads(result)

        for account in posixAccounts.get('posixAccounts', []):
            if 'accountId' in account:
                self.project_id = account['accountId']
                self.username = account['username']

    def set_deployment_variables(self):
        self.workspace = os.path.abspath(os.getcwd().strip())
        self.parse_blueprint(self.blueprint_yaml)
        self.get_posixAccount_info()
        self.instance_name = self.deployment_name.replace("-", "")[:10] + "-slurm-login-001"

    def create_blueprint(self):
        cmd = [
              "./gcluster",
              "create",
              "-l",
              "ERROR",
              self.blueprint_yaml,
              "--backend-config",
              f"bucket={self.state_bucket}",
              "--vars",
              f"project_id={self.project_id}",
              "--vars",
              f"deployment_name={self.deployment_name}",
              "-w"
          ]

        subprocess.run(cmd, check=True, cwd=self.workspace)

    def compress_blueprint(self):        
        cmd = [
              "tar", 
              "-czf",
              "%s.tgz" % (self.deployment_name),
              "%s" % (self.deployment_name),
          ]

        subprocess.run(cmd, check=True, cwd=self.workspace)

    def upload_deployment(self):
        cmd = [
              "gsutil",
              "cp",
              "%s.tgz" % (self.deployment_name),
              "gs://%s/%s/" % (self.state_bucket, self.deployment_name)
          ]

        subprocess.run(cmd, check=True, cwd=self.workspace)

    def print_download_command(self):
        print("gcloud storage cp gs://%s/%s/%s.tgz ." % (self.state_bucket, self.deployment_name, self.deployment_name))

    def create_deployment_directory(self):
        self.set_deployment_variables()
        self.create_blueprint()
        self.compress_blueprint()
        self.upload_deployment()
        self.print_download_command()

    def deploy(self):
        # Create deployment directory
        self.create_deployment_directory()
        cmd = [
              "./gcluster",
              "deploy",
              self.deployment_name,
              "--auto-approve"
          ]

        subprocess.run(cmd, check=True, cwd=self.workspace)

    def destroy(self):
        cmd = [
              "./gcluster",
              "destroy",
              self.deployment_name,
              "--auto-approve"
          ]

        subprocess.run(cmd, check=True, cwd=self.workspace)
        os.remove(f"{self.deployment_name}.tgz")
        shutil.rmtree(self.deployment_name)
