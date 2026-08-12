#!/usr/bin/env python3
# Copyright 2026 Google LLC
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

from http.server import ThreadingHTTPServer, BaseHTTPRequestHandler
import logging
import os
import shutil
import socket
import subprocess
import sys
import urllib.request

from typing import Optional

PORT = 6821

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    handlers=[logging.StreamHandler(sys.stdout)],
)
log = logging.getLogger("slurm_health_check")


def find_scontrol():
    """Find the scontrol binary path."""
    for path in ["/usr/local/bin/scontrol", "/usr/bin/scontrol", "/bin/scontrol"]:
        if os.path.exists(path) and os.access(path, os.X_OK):
            return path
    scontrol = shutil.which("scontrol")
    if scontrol:
        return scontrol
    return "/usr/local/bin/scontrol"


SCONTROL_PATH = find_scontrol()


class HealthCheckHandler(BaseHTTPRequestHandler):
    _is_backup: Optional[bool] = None

    def log_message(self, format, *args):
        # Suppress logging of every routine GET probe to prevent log spam
        pass

    def do_GET(self):
        hostname = socket.gethostname().split(".")[0]
        try:
            # Timeout is 3s (lower than 5s LB probe timeout to prevent race condition/TCP teardown)
            res = subprocess.run(
                [SCONTROL_PATH, "ping"],
                capture_output=True,
                text=True,
                timeout=3,
                check=False,
            )
            output = (res.stdout + res.stderr).lower()
        except Exception as e:
            log.error(f"Health check probe failed when executing scontrol ping: {e}")
            self.send_response(503)
            self.end_headers()
            self.wfile.write(f"Failed to run scontrol ping: {e}\n".encode())
            return

        primary_up = any(
            "primary controller: up" in line
            or "primary controller(up)" in line
            or ("slurmctld(primary) at" in line and "is up" in line)
            for line in output.splitlines()
        )
        backup_up = any(
            "backup controller: up" in line
            or "backup controller(up)" in line
            or ("slurmctld(backup) at" in line and "is up" in line)
            for line in output.splitlines()
        )

        # Active-passive role determination based on hostname convention (-0 vs -1) and cached metadata fallback
        # Note: Role is cached once determined, but health probe failures are not cached so subsequent probes retry on every request.
        is_backup = getattr(HealthCheckHandler, "_is_backup", None)
        if is_backup is None:
            if hostname.endswith("-1"):
                is_backup = True
            elif hostname.endswith("-0"):
                is_backup = False
            else:
                # If hostname does not follow -0 / -1 convention, query instance metadata as fallback
                try:
                    # Query instance name first as it is guaranteed to have the -0 / -1 suffix in GCP
                    req_name = urllib.request.Request(
                        "http://metadata.google.internal/computeMetadata/v1/instance/name",
                        headers={"Metadata-Flavor": "Google"}
                    )
                    with urllib.request.urlopen(req_name, timeout=1) as resp:
                        inst_name = resp.read().decode().strip()
                        if inst_name.endswith("-1"):
                            is_backup = True
                        elif inst_name.endswith("-0"):
                            is_backup = False
                        else:
                            # Fallback to slurm_ha_role attribute
                            req_role = urllib.request.Request(
                                "http://metadata.google.internal/computeMetadata/v1/instance/attributes/slurm_ha_role",
                                headers={"Metadata-Flavor": "Google"}
                            )
                            with urllib.request.urlopen(req_role, timeout=1) as resp_role:
                                role = resp_role.read().decode().strip()
                                if role == "backup":
                                    is_backup = True
                                elif role == "primary":
                                    is_backup = False
                                else:
                                    # Align with setup.py: assume backup if neither hostname nor inst_name ends with -0
                                    is_backup = not (hostname.endswith("-0") or inst_name.endswith("-0"))
                except Exception as e:
                    log.error(f"Failed to resolve role from metadata: {e}")
                    self.send_response(503)
                    self.end_headers()
                    self.wfile.write(f"Failed to resolve role: {e}\n".encode())
                    return
            HealthCheckHandler._is_backup = is_backup

        if not is_backup:
            if primary_up:
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b"OK - Primary Active\n")
            else:
                self.send_response(503)
                self.end_headers()
                self.wfile.write(b"Primary Offline\n")
        else:
            if not primary_up and backup_up:
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b"OK - Backup Active (Took Over)\n")
            else:
                self.send_response(503)
                self.end_headers()
                self.wfile.write(b"Standby Backup\n")


class ReusableHTTPServer(ThreadingHTTPServer):
    allow_reuse_address = True


def main():
    log.info(
        f"Starting Slurm HA HTTP Health Check agent on port {PORT} using scontrol at {SCONTROL_PATH}"
    )
    server = ReusableHTTPServer(("0.0.0.0", PORT), HealthCheckHandler)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        log.info("Shutting down health check agent.")
        server.server_close()


if __name__ == "__main__":
    main()
