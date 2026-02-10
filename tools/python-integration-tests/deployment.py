# Copyright 2026 "Google LLC"
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
import uuid
import copy
import tempfile

class Deployment:
    def __init__(self, blueprint: str):
        self.blueprint_file = blueprint
        self.state_bucket = "daily-tests-tf-state"
        self.project_id = None
        self.workspace = None
        self.username = None
        self.deployment_name = None
        self.blueprint_name = None
        self.zone = None

    def run_command(self, cmd: str, err_msg: str = None) -> subprocess.CompletedProcess:
        res = subprocess.run(cmd, shell=True, universal_newlines=True, check=True,
                         stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        return res

    def parse_blueprint(self, file_path: str):
        with open(file_path, 'r') as file:
            content = yaml.safe_load(file)
        self.zone = content["vars"]["zone"]
        self.blueprint_name = content["blueprint_name"]

    def get_posixAccount_info(self):
        # Extract the username from posixAccounts
        cmd = "gcloud compute os-login describe-profile --format=json"
        result = self.run_command(cmd).stdout
        posixAccounts = json.loads(result)

        for account in posixAccounts.get('posixAccounts', []):
            if 'accountId' in account:
                return account['accountId'], account['username']
        raise RuntimeError(f"Can not find a project in `{cmd}`")

    def generate_uniq_deployment_name(self):
        BUILD_ID = os.environ.get('BUILD_ID')
        prefix = (BUILD_ID if BUILD_ID else str(uuid.uuid4()))[:6]
        return f"{prefix}-{self.blueprint_name}"

    def set_deployment_variables(self):
        self.workspace = os.path.abspath(os.getcwd().strip())
        self.parse_blueprint(self.blueprint_file)
        self.deployment_name = self.generate_uniq_deployment_name()
        self.project_id, self.username = self.get_posixAccount_info()

    def create_blueprint(self):
        cmd = [
              "./gcluster",
              "create",
              "-l",
              "ERROR",
              self.blueprint_file,
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

    def generate_blueprint_with_spot_config(self, enable_spot: bool) -> str:
        """
        Generates a temporary blueprint file with the specified Spot VM configuration.
        Returns the path to the temporary blueprint file.
        """
        config = None
        with open(self.blueprint_file, 'r') as f:
            config = yaml.safe_load(f)

        # Iterate through groups and modules to find nodesets
        for group in config.get('deployment_groups', []):
            for module in group.get('modules', []):
                source = module.get('source', '')
                if 'nodeset' in source:
                    if 'settings' not in module:
                        module['settings'] = {}
                    
                    module['settings']['enable_spot_vm'] = enable_spot
                    print(f"Set enable_spot_vm={enable_spot} for module {module.get('id')}")

        fd, temp_path = tempfile.mkstemp(suffix='.yaml', text=True)
        with os.fdopen(fd, 'w') as f:
            yaml.dump(config, f)
        
        return temp_path

    def deploy(self):
        """
        Deploys the blueprint using a waterfall strategy:
        Tier 1: Spot VMs (enable_spot_vm=True)
        Tier 2: On-Demand VMs (enable_spot_vm=False)
        """
        original_blueprint_file = self.blueprint_file
        
        tiers = [
            (True, "Tier 1: Spot VMs"),
            (False, "Tier 2: On-Demand VMs")
        ]

        for i, (enable_spot, description) in enumerate(tiers):
            print(f"\n--- Attempting {description} ---")
            
            try:
                temp_blueprint = self.generate_blueprint_with_spot_config(enable_spot)
                self.blueprint_file = temp_blueprint
                
                self.create_deployment_directory()
                
                cmd = [
                      "./gcluster",
                      "deploy",
                      self.deployment_name,
                      "--auto-approve"
                  ]
                
                subprocess.run(cmd, check=True, cwd=self.workspace)
                
                print(f"Successfully deployed using {description}")
                return

            except Exception as e:
                print(f"Deployment failed for {description}: {e}")
                if i == len(tiers) - 1:
                    print("All provisioning tiers failed.")
                    raise e
                
                print("Falling back to next tier...")
                
                try:
                    self.destroy()
                except Exception as destroy_err:
                    print(f"Warning: Failed to cleanup after failed deployment: {destroy_err}")

            finally:
                self.blueprint_file = original_blueprint_file

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
