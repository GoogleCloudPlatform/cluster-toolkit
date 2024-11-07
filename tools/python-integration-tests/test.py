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
import logging
import shutil
import os
import re
import signal
import socket
import subprocess
import sys
import time
import paramiko
from collections import defaultdict
import argparse
import yaml

def run_command(cmd: str, err_msg: str = None) -> subprocess.CompletedProcess:
        res = subprocess.run(cmd, shell=True, universal_newlines=True, check=True,
                         stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if res.returncode != 0:
            raise subprocess.SubprocessError(f"{err_msg}:\n{res.stderr}")
       
        return res

def parse_blueprint(file_path: str):
    with open(file_path, 'r') as file:
        content = yaml.safe_load(file)
    return content["vars"]["deployment_name"], content["vars"]["zone"]

def get_account_info():
    # Extract the username from posixAccounts
    result = run_command(f"gcloud compute os-login describe-profile --format=json").stdout
    posixAccounts = json.loads(result)

    for account in posixAccounts.get('posixAccounts', []):
        if 'accountId' in account:
            project_id = account['accountId']
            username = account['username']
    return project_id, username

def create_deployment(blueprint: str):
    project_id, username = get_account_info()
    deployment_name, zone = parse_blueprint(blueprint)
    return Deployment(blueprint, project_id, username, deployment_name, zone)

def test_simple_job_completion(blueprint: str):    
    deployment = create_deployment(blueprint) 
    deployment.deploy()
    try:
        # Waiting to let the login node finish set up or ssh will fail.
        print("Wait 60 seconds")
        time.sleep(60)

        ssh = deployment.ssh()
        test = Test(ssh, deployment)
        test.check_simple_job_completion()
    finally:
        deployment.close_tunnel()
        deployment.destroy()

def test_topology(blueprint: str):    
    deployment = create_deployment(blueprint) 
    deployment.deploy()
    try:
        # Waiting to let the login node finish set up or ssh will fail.
        print("Wait 60 seconds")
        time.sleep(60)
        ssh = deployment.ssh()
        test = Test(ssh, deployment)
        test.check_topology()
    finally:
        deployment.close_tunnel()
        deployment.destroy()

class Deployment:
    def __init__(self, blueprint: str, project_id: str, username: str, deployment_name: str, zone: str):
        self.blueprint_yaml = blueprint
        self.project_id = project_id
        self.state_bucket = "daily-tests-tf-state"
        self.workspace = ""
        self.username = username
        self.deployment_name = deployment_name
        self.zone = zone
        self.test_name = deployment_name
        self.tunnel = None

    def get_workspace(self):
        return os.path.abspath(os.getcwd().strip())

    def create_blueprint(self):
        self.workspace = self.get_workspace()

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
              f"deployment_name={self.deployment_name}"
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
              "gs://%s/%s/" % (self.state_bucket, self.test_name)
          ]

        subprocess.run(cmd, check=True, cwd=self.workspace)

    def print_download_command(self):
        print("gcloud storage cp gs://%s/%s/%s.tgz ." % (self.state_bucket, self.test_name, self.deployment_name))

    def create_deployment_directory(self):
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

    def ssh(self) -> paramiko.SSHClient:
        instance_name = self.deployment_name.replace("-", "")[:10] + "-slurm-login-001"

        # Use existing SSH key pair (assuming it's already in ~/.ssh/google_compute_engine)
        key_path = os.path.expanduser("~/.ssh/google_compute_engine")

        # Add the public key to OS Login
        public_key_path = key_path + ".pub"
        subprocess.run(
            [
                "gcloud", "compute", "os-login", "ssh-keys", "describe", 
                "--key-file", public_key_path
            ], 
            check=True, capture_output=True
        )

        # Construct the gcloud command to create the IAP tunnel
        iap_tunnel_cmd = [
            "gcloud", "compute", "start-iap-tunnel", instance_name,
            "22", "--project", self.project_id, "--zone", self.zone,
            "--local-host-port=localhost:10022"
        ]

        # Create the IAP tunnel process
        self.tunnel = subprocess.Popen(iap_tunnel_cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        
        # Sleep to give the tunnel a few seconds to set up
        time.sleep(3)

        # Create an SSH client
        ssh = paramiko.SSHClient()
        ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

        # Load the private key
        key = paramiko.RSAKey.from_private_key_file(key_path)

        # Connect to the VM 
        ssh.connect("localhost", port=10022, username=self.username, pkey=key)
    
        return ssh
        
    def close_tunnel(self):
        if self.tunnel:
            self.tunnel.terminate()
            self.tunnel.wait()
            self.tunnel = None

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


class Test:
    def __init__(self, ssh, deployment):
        self.ssh = ssh
        self.deployment = deployment
        self.job_list = {}

    def get_slurm_topology(self):
        stdin, stdout, stderr = self.ssh.exec_command("scontrol show topo")
        return stdout.read().decode() 

    def monitor_squeue(self):
        # Monitors squeue and updates self.job_list until all running jobs are complete.
        lines = []

        while True:
            stdin, stdout, stderr = self.ssh.exec_command('squeue')

            lines = stdout.read().decode().splitlines()[1:] # Skip header

            if not lines:
                break
            for line in lines:
                parts = line.split()
                job_id, partition, _, _, state, times, nodes, nodelist = line.split()

                if job_id not in self.job_list:
                    print(f"Job id {job_id} is not recognized.")
                else:
                    self.job_list[job_id].update({
                        "partition": partition,
                        "state": state,
                        "time": times,
                        "nodes": nodes,
                        "nodelist": nodelist,
                    })
            time.sleep(5)

    def is_job_complete(self, job_id: str):
        # Checks if a job successfully completed.
        stdin, stdout, stderr = self.ssh.exec_command(f'scontrol show job {job_id} --json')
        content = json.load(stdout)
        return content["jobs"][0]["job_state"][0] == "COMPLETED"

    def submit_job(self, cmd: str):
        stdin, stdout, stderr = self.ssh.exec_command(cmd)
        jobID = stdout.read().decode().split()[-1]
        self.job_list[jobID] = {}

    def get_node_depth(self, switch_name: str):
        return switch_name.count("_")

    def get_real_rack(self, node: str):
        result = run_command(f"gcloud compute instances describe {node} --zone={self.deployment.zone} --project={self.deployment.project_id} --format='value(resourceStatus.physicalHost)'")
        return result.stdout.split("/")[1]
    
    def get_slurm_rack(self, node: str):
        stdin, stdout, stderr = self.ssh.exec_command(f"scontrol show topology {node} | tail -1 | cut -d' ' -f1")
        switch_name = stdout.read().decode()
        self.assert_equal(self.get_node_depth(switch_name), 2, f"{node} does not have the expected topology depth of 2."),
        return switch_name

    def get_nodes(self):
        nodes = []
        stdin, stdout, stderr = self.ssh.exec_command("scontrol show node| grep NodeName")
        for line in stdout.read().decode().splitlines():
            nodes.append(line.split()[0].split("=")[1])
        return nodes

    def assert_equal(self, value1, value2, message=None):
        if value1 != value2:
            if message is None:
                message = f"Assertion failed: {value1} != {value2}"
            raise AssertionError(message)

    def check_simple_job_completion(self):
        # Submits 5 jobs and checks if they are successful.
        for i in range(5):
            self.submit_job('sbatch -N 1 --wrap "sleep 20"')
        self.monitor_squeue()

        for job_id in self.job_list.keys():
            result = self.is_job_complete(job_id)
            self.assert_equal(True, result, f"Something went wrong with JobID:{job_id}.")
            print(f"JobID {job_id} finished successfully.")
        
    def check_topology(self):
        # Checks isomorphism of last layer of nodes to determine topology.
        r_rack, s_rack = defaultdict(set), defaultdict(set)
        nodes = self.get_nodes()

        for node in nodes:
            r_rack[self.get_real_rack(node)].add(node)
            s_rack[self.get_slurm_rack(node)].add(node)

        r_rack_set = [set(v) for v in r_rack.values()]
        s_rack_set = [set(v) for v in s_rack.values()]   

        self.assert_equal(r_rack_set, s_rack_set, "The two sets did not match.")

def main(simple_test_blueprints, topo_test_blueprints) -> None:
    if simple_test_blueprints:
        for blueprint in simple_test_blueprints:
            test_simple_job_completion(blueprint)
            print(f'{blueprint} passed simple slurm test.')
    
    if topo_test_blueprints:
        for blueprint in topo_test_blueprints:
            test_topology(blueprint)
            print(f'{blueprint} passed topology test.')

if __name__ == "__main__":
    parser = argparse.ArgumentParser(
        prog='test.py',
        description="",
        formatter_class=argparse.RawTextHelpFormatter
    )
    parser.add_argument("--simple", nargs="+", help="File path(s) to blueprint(s) to do the simple slurm test on.")
    parser.add_argument("--topo", nargs="+", help="File path(s) to blueprint(s) to do the topology test on.")

    args = parser.parse_args()

    main(args.simple, args.topo)
