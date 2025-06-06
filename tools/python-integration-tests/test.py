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

import subprocess
import unittest
import re
import ssh
from deployment import Deployment

import logging
log = logging.getLogger()

class Test(unittest.TestCase):  # Inherit from unittest.TestCase
    def __init__(self, deployment):
        super().__init__()  # Call the superclass constructor
        self.deployment = deployment
        self.ssh_mngr = None
        

    def run_command(self, cmd: str) -> subprocess.CompletedProcess:
        res = subprocess.run(cmd, shell=True, text=True, check=True,
                    stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        return res

    def setUp(self):
        self.addCleanup(self.deployment.destroy)
        self.addCleanup(lambda: self.ssh_mngr.close() if self.ssh_mngr else None)

        self.deployment.deploy()
        self.ssh_mngr = ssh.SSHManager(self.deployment.username, self.deployment.project_id, self.deployment.zone)
        
class SlurmTest(Test):
    def __init__(self, deployment: Deployment):
        super().__init__(deployment)

    def ssh_login(self):
        host = re.sub(r"^[^a-z]*|[^a-z0-9]", "", self.deployment.deployment_name)[:10] + "-slurm-login-001"
        return self.ssh_mngr.ssh(host)

    def get_nodes(self):
        nodes = []
        stdout = ssh.exec_and_check(self.ssh_login(), "scontrol show node| grep NodeName")
        for line in stdout.splitlines():
            nodes.append(line.split()[0].split("=")[1])
        return nodes
    
    def setUp(self):
        super().setUp()
        self.wait_for_setup()

    def wait_for_setup(self):
        log.info("Waiting for login node setup:")
        timeout = 5 * 60 # 5 minutes
        sess = self.ssh_login()
        _, stdout, _ = sess.exec_command('sudo tail -f -n +1 /slurm/scripts/log/setup.log', get_pty=True, timeout=timeout)

        for line in stdout:
            log.info(f"setup.log: {line.rstrip()}")
            if "Done setting up" in line:
                stdout.channel.close()
            if "Aborting setup..." in line:
                stdout.channel.close()
                raise ValueError("Setup failed")
