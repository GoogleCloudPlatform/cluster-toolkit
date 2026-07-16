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

import http.server
import logging
import os
import shutil
import socket
import subprocess
import sys

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


class HealthCheckHandler(http.server.BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        # Suppress logging of every routine GET probe to prevent log spam
        pass

    def do_GET(self):
        hostname = socket.gethostname().split(".")[0]
        try:
            res = subprocess.run(
                [SCONTROL_PATH, "ping"],
                capture_output=True,
                text=True,
                timeout=5,
                check=False,
            )
            output = (res.stdout + res.stderr).lower()
        except Exception as e:
            log.error(f"Health check probe failed when executing scontrol ping: {e}")
            self.send_response(503)
            self.end_headers()
            self.wfile.write(f"Failed to run scontrol ping: {e}\n".encode())
            return

        primary_up = (
            "primary controller: up" in output
            or "primary controller(up)" in output
            or "slurmctld(primary) at" in output
            and "is up" in output
        )
        backup_up = (
            "backup controller: up" in output
            or "backup controller(up)" in output
            or "slurmctld(backup) at" in output
            and "is up" in output
        )

        # Active-passive role determination based on hostname convention (-0 vs -1)
        if hostname.endswith("-0"):
            if primary_up:
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b"OK - Primary Active\n")
            else:
                self.send_response(503)
                self.end_headers()
                self.wfile.write(b"Primary Offline\n")
        elif hostname.endswith("-1"):
            if not primary_up and backup_up:
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b"OK - Backup Active (Took Over)\n")
            else:
                self.send_response(503)
                self.end_headers()
                self.wfile.write(b"Standby Backup\n")
        else:
            # Fallback for dynamic/non-standard hostname naming: check if scontrol ping succeeded at all
            if primary_up or backup_up:
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b"OK - Controller Active\n")
            else:
                self.send_response(503)
                self.end_headers()
                self.wfile.write(b"Controller Offline\n")


class ReusableHTTPServer(http.server.HTTPServer):
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
