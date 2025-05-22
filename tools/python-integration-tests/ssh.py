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
logging.getLogger("paramiko").setLevel("INFO")

@functools.lru_cache()
def init_key() -> str:
    key_path = os.path.expanduser("~/.ssh/slurm_tests")
    os.makedirs(os.path.dirname(key_path), exist_ok=True)

    if os.path.exists(key_path):
        log.info(f"{key_path=} already exists, reusing")
    else:
        subprocess.run(["ssh-keygen", "-t", "rsa", "-f", key_path, "-N", ""], check=True)

    # Add the public key to OS Login
    public_key_path = key_path + ".pub"
    subprocess.run(["gcloud", "compute", "os-login", "ssh-keys", "add", "--key-file", public_key_path, "--ttl", "60m"], check=True, stdout=subprocess.DEVNULL)
    return key_path


def find_open_port():
    while True:
        with contextlib.closing(socket.socket(socket.AF_INET, socket.SOCK_STREAM)) as s:
            s.bind(("localhost", 0))
            s.listen(1)
            s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            port = s.getsockname()[1]
        yield port

class Tunnel:
    def __init__(self, host:str, project:str, zone:str, target_port:int=22):
        self.host = host
        self.target_port = target_port
        self.port, self.proc = -1, None # pre-init to account for failed `_start_tunnel`
        self.port, self.proc = self._start_tunnel(host, project, zone, target_port)

    def __del__(self) -> None:
        self.close()

    def __repr__(self) -> str:
        if self.proc:
            return f"Tunnel({self.port}:{self.host}:{self.target_port}<{self.proc.pid}>)"
        return f"Tunnel({self.port}:{self.host}:{self.target_port}<closed>)"

    def close(self) -> None:
        if not self.proc:
            return
        if log:
            log.info(f"closing {self}")
        self.proc.terminate()
        if time:
            time.sleep(1) # give a second to terminate
        if self.proc.poll() is None: 
            self.proc.kill() # kill leftover process if still running
        # TODO: shall we recursively kill children?


    def _start_tunnel(self, instance:str, project:str, zone:str,  target_port:int):
        listen = re.compile(r"^Listening on port \[\d+\].\n$")
        log.info(f"start tunnel {instance}:{target_port}")

        def tunnel(port: int):
            """Attempt to create an iap tunnel on the local port"""
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
                            return proc
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

        for port in find_open_port():
            try:
                t = tunnel(port)
            except Exception:
                log.exception("tunnel creation failed")
                raise
            if t is not None:
                return port, t
        raise RuntimeError("No available port found")

class SSHManager:
    def __init__(self, user: str, project: str, zone: str) -> None:
        self.user = user
        self.project = project
        self.zone = zone
        self.ssh_conns = {}
        self.tunnels = {}

    def close(self) -> None:
        for hostname in list(self.ssh_conns.keys()):
            ssh = self.ssh_conns.get(hostname)
            if ssh:
                ssh.close()
                del self.ssh_conns[hostname]

        for hostname in list(self.tunnels.keys()):
            tunnel = self.tunnels.get(hostname)
            if tunnel:
                tunnel.close()
                del self.tunnels[hostname]

    def tunnel(self, hostname: str) -> Tunnel:
        if hostname not in self.tunnels:
            self.tunnels[hostname] = Tunnel(hostname, self.project, self.zone)
        return self.tunnels[hostname]


    def ssh(self, hostname: str) -> paramiko.SSHClient:
        if hostname in self.ssh_conns:
            return self.ssh_conns[hostname]

        ssh = paramiko.SSHClient()
        key = paramiko.RSAKey.from_private_key_file(init_key())
        ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        for wait in [0.5, 1, 2, 4, 8, 16, 32, 64, 128]:  # exponential backoff
            tun = self.tunnel(hostname)
            log.info(f"start ssh connection to {self.user}@{hostname} port {tun.port}")
            try:
                ssh.connect("127.0.0.1", username=self.user, pkey=key, port=tun.port)
                break
            except paramiko.ssh_exception.NoValidConnectionsError:
                log.error("ssh connection failed, retrying tunnel")
                time.sleep(wait)
                tun = self.tunnels.pop(hostname)
                tun.close()
                continue
            except Exception as e:
                log.error(f"error on start ssh connection: {e}")
        else:
            log.error(f"Cannot connect through tunnel: {hostname}")
            raise Exception(f"Cannot connect through tunnel: {hostname}")
        self.ssh_conns[hostname] = ssh
        return ssh


def exec_and_check(ssh: paramiko.SSHClient, cmd: str) -> str:
    _, stdout, stderr = ssh.exec_command(cmd)
    rc = stdout.channel.recv_exit_status()
    if rc != 0:
        raise RuntimeError(f"'{cmd}' exited with code {rc}: {stderr.read().decode()}")
    return stdout.read().decode()
