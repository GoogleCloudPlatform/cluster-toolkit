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
import subprocess
import time
import unittest
from ssh import SSHManager
from deployment import Deployment

class Test(unittest.TestCase):  # Inherit from unittest.TestCase
    def __init__(self, deployment):
        super().__init__()  # Call the superclass constructor
        self.deployment = deployment
        self.ssh_manager = None
        self.ssh_client = None

    def run_command(self, cmd: str) -> subprocess.CompletedProcess:
        res = subprocess.run(cmd, shell=True, text=True, check=True,
                    stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        return res

    def setUp(self):
        self.addCleanup(self.clean_up)
        self.deployment.deploy()
        time.sleep(90)

    def clean_up(self):
        self.deployment.destroy()

class SlurmTest(Test):
    # Base class for Slurm-specific tests.
    def ssh(self, hostname):
        self.ssh_manager = SSHManager()
        self.ssh_manager.setup_connection(hostname, 10022, self.deployment.project_id, self.deployment.zone)
        self.ssh_client = self.ssh_manager.ssh_client
        self.ssh_client.connect("localhost", 10022, username=self.deployment.username, pkey=self.ssh_manager.key)

    def close_ssh(self):
        self.ssh_manager.close()

    def setUp(self):
        try:
            super().setUp()
            hostname = self.get_login_node()
            self.ssh(hostname)
        except Exception as err:
            self.fail(f"Unexpected error encountered. stderr: {err.stderr}")

    def clean_up(self):
        super().clean_up()
        self.close_ssh()

    def get_login_node(self):
        return self.deployment.deployment_name.replace("-", "")[:10] + "-slurm-login-001"

    def assert_equal(self, value1, value2, message=None):
        if value1 != value2:
            if message is None:
                message = f"Assertion failed: {value1} != {value2}"
            raise AssertionError(message)

    def get_nodes(self):
        nodes = []
        stdin, stdout, stderr = self.ssh_client.exec_command("scontrol show node| grep NodeName")
        for line in stdout.read().decode().splitlines():
            nodes.append(line.split()[0].split("=")[1])
        return nodes
