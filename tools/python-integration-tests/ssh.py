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

import os
import subprocess
import time
import paramiko

class SSHManager:
    # Manages tunnel and SSH connection.
    _instance = None

    def __new__(cls, *args, **kwargs):
       if not cls._instance:
           cls._instance = super(SSHManager, cls).__new__(cls)
       return cls._instance

    def __init__(self):
        if not hasattr(self, 'ssh_client'):
            self.tunnel = None
            self.key = None
            self.ssh_client = None

    def run_command(self, cmd: str) -> subprocess.CompletedProcess:
        res = subprocess.run(cmd, text=True, check=True, capture_output=True)

    def create_tunnel(self, instance_name, port, project_id, zone):
        iap_tunnel_cmd = [
            "gcloud", "compute", "start-iap-tunnel", instance_name,
            "22", "--project", project_id, "--zone", zone,
            f"--local-host-port=localhost:{port}"
        ]

        self.tunnel = subprocess.Popen(iap_tunnel_cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        
        # Sleep to give the tunnel a few seconds to set up
        time.sleep(3)

    def get_keypath(self):
        key_path = os.path.expanduser("~/.ssh/google_compute_engine")
        os.makedirs(os.path.dirname(key_path), exist_ok=True)

        self.run_command(["ssh-keygen", "-t", "rsa", "-f", key_path, "-N", ""])

        # Add the public key to OS Login
        public_key_path = key_path + ".pub"
        self.run_command(["gcloud", "compute", "os-login", "ssh-keys", "add", "--key-file", public_key_path, "--ttl", "60m"])

        return key_path

    def setup_connection(self, instance_name, port, project_id, zone):
        self.ssh_client = paramiko.SSHClient()
        self.ssh_client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        self.key = paramiko.RSAKey.from_private_key_file(self.get_keypath())
        self.create_tunnel(instance_name, port, project_id, zone)

    def close(self):
        # Closes existing SSH connection and tunnel
        if self.ssh_client:
            self.ssh_client.close()
        if self.tunnel:
            self.tunnel.terminate()
            time.sleep(1) # give a second to terminate
            if self.tunnel.poll() is None: 
                self.tunnel.kill() # kill leftover process if still running
            self.tunnel.stdout.close()
            self.tunnel.stderr.close()
            self.tunnel = None
