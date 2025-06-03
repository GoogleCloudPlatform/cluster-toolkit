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

import typing
import os
import subprocess
import socket
import time
import paramiko
import logging
import re
import select
import pty
import contextlib
import functools
import shlex

log = logging.getLogger()

def get_available_port() -> int:
    with contextlib.closing(socket.socket(socket.AF_INET, socket.SOCK_STREAM)) as s:
        s.bind(("localhost", 0))
        s.listen(1)
        s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        return s.getsockname()[1]

def _try_create_tunnel(instance:str, project:str, zone:str,  target_port:int, port: int) -> typing.Optional[subprocess.Popen]:
    listen = re.compile(r"^Listening on port \[\d+\].\n$")
    log.info(f"start tunnel {instance}:{target_port}")

    # the pty makes gcloud output a message on success, allowing us to
    # proceed faster
    stdoutfd, peer = pty.openpty()
    stdout = os.fdopen(stdoutfd)
    cmd = f"gcloud compute start-iap-tunnel {instance} {target_port} --{project=} --{zone=} --local-host-port=localhost:{port}"
    log.info(f"Running {cmd}")
    proc = subprocess.Popen(
        shlex.split(cmd),
        shell=False,
        text=True,
        stderr=subprocess.PIPE,
        stdout=peer,
        stdin=subprocess.DEVNULL,
    )
    stdout_sel = select.poll()
    stdout_sel.register(stdout, select.POLLIN)
    for w in [0.5, 1, 2, 4, 8, 16]:  # exponential backoff
        if proc.poll() is None:
            if stdout_sel.poll(1):
                out = stdout.readline()
                log.info(f"gcloud iap-tunnel: {out}")
                if listen.match(out):
                    log.info(f"gcloud iap-tunnel created on port {port}")
                    stdout.close()
                    os.close(peer)
                    return proc, port
        else:
            stderr = proc.stderr.read() if proc.stderr else ""
            if "Could not fetch resource" in stderr:
                raise RuntimeError(f"Tunnel failed with unrecoverable error: {stderr}")
            log.info(f"gcloud iap-tunnel failed on {port=}, rc={proc.returncode}, {stderr=}")
            return None # to be retried
        time.sleep(w)
    log.error(f"gcloud iap-tunnel timed out on port {port}")
    proc.kill()
    return None

def create_tunnel(instance:str, project:str, zone:str,  target_port:int) -> tuple[subprocess.Popen, int]:
    for w in [0.5, 1, 2, 4, 8, 16]:  # exponential backoff
        port =  get_available_port()
        t = _try_create_tunnel(instance, project, zone, target_port, port)
        if t:
            return t, port
        time.sleep(w)
    raise RuntimeError("Failed to create tunnel")

def close_tunnel(proc: typing.Optional[subprocess.Popen]) -> None:
    if proc is None:
        return
    proc.terminate()
    time.sleep(1) # give a second to terminate
    if proc.poll() is None: 
        proc.kill() # kill leftover process if still running
    if proc.stdout:
        proc.stdout.close()
        proc.stderr.close()
    

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
            self.local_port = None

    def get_keypath(self):
        key_path = os.path.expanduser("~/.ssh/slurm_tests")
        os.makedirs(os.path.dirname(key_path), exist_ok=True)

        if os.path.exists(key_path):
            pass
        else:
            subprocess.run(["ssh-keygen", "-t", "rsa", "-f", key_path, "-N", ""], check=True)

        # Add the public key to OS Login
        public_key_path = key_path + ".pub"
        subprocess.run(["gcloud", "compute", "os-login", "ssh-keys", "add", "--key-file", public_key_path, "--ttl", "60m"], check=True, stdout=subprocess.DEVNULL)

        return key_path

    def setup_connection(self, instance_name, project_id, zone):
        self.ssh_client = paramiko.SSHClient()
        self.ssh_client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        self.key = paramiko.RSAKey.from_private_key_file(self.get_keypath())
        self.tunnel, self.local_port = create_tunnel(instance_name, project_id, zone, target_port=22)

    def close(self):
        # Closes existing SSH connection and tunnel
        if self.ssh_client:
            self.ssh_client.close()
        close_tunnel(self.tunnel)


def exec_and_check(ssh: paramiko.SSHClient, cmd: str) -> str:
    _, stdout, stderr = ssh.exec_command(cmd)
    rc = stdout.channel.recv_exit_status()
    if rc != 0:
        raise RuntimeError(f"'{cmd}' exited with code {rc}: {stderr.read().decode()}")
    return stdout.read().decode()
