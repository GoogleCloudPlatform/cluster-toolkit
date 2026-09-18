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

"""Running things as the session's user, and preparing their environment."""

import logging
import os
import shlex
import subprocess
from pathlib import Path

from ..errors import BrokerError

LOG = logging.getLogger("ghpc-desktop-broker")


class UserEnvironment:
    """Everything the broker does on behalf of, or to, a user's account."""

    def __init__(self, runtime_dir, vnc_backend):
        self.runtime_dir = runtime_dir
        self.vnc_backend = vnc_backend

        self.runtime_dir.mkdir(parents=True, exist_ok=True)
        # Traversable, so each user can reach their own 0700 subdirectory.
        os.chmod(self.runtime_dir, 0o755)

    def run_as(
        self,
        username,
        home_dir,
        args,
        check=True,
        environment=None,
        input_text=None,
        capture_output=False,
    ):
        env_args = [
            "env",
            f"HOME={home_dir}",
            f"USER={username}",
            f"LOGNAME={username}",
            "SHELL=/bin/bash",
        ]
        for key, value in sorted((environment or {}).items()):
            env_args.append(f"{key}={value}")
        command = ["runuser", "-u", username, "--", *env_args, *args]
        return subprocess.run(
            command,
            check=check,
            input=input_text,
            capture_output=capture_output,
            text=capture_output or input_text is not None,
        )

    def xstartup_contents(self):
        """The ~/.vnc/xstartup this broker writes.

        Sources /etc/profile before starting the desktop. On a Slurm login node
        that is what puts sbatch, environment modules and a non-default
        SLURM_CONF into the session - a non-login runuser invocation inherits
        none of it. Doing it here rather than leaving it to the terminal also
        covers GUI applications launched from the desktop menu, which is what a
        pre- or post-processor shelling out to srun needs.

        Guarded and non-fatal: a profile script that exits non-zero must not
        stop the desktop from starting.
        """
        exports = "".join(
            f"export {key}={shlex.quote(str(value))}\n"
            for key, value in sorted(
                self.vnc_backend.session_environment().items()
            )
        )
        argv = " ".join(
            shlex.quote(part) for part in self.vnc_backend.session_command()
        )
        return (
            "#!/bin/sh\n"
            "unset SESSION_MANAGER\n"
            "unset DBUS_SESSION_BUS_ADDRESS\n"
            "if [ -r /etc/profile ]; then\n"
            "  . /etc/profile || true\n"
            "fi\n"
            f"{exports}"
            f"exec {argv}\n"
        )

    def ensure_home(self, username, home_dir, uid, gid):
        home_path = Path(home_dir)
        try:
            # Only the home directory is created as root, since its parent is
            # not user-writable. Everything below it is created by the user, so
            # a symlinked .vnc or xstartup cannot redirect a root write or
            # chown.
            if not home_path.exists():
                home_path.mkdir(parents=True, exist_ok=True)
                os.chown(home_path, uid, gid)
                os.chmod(home_path, 0o700)
            vnc_dir = home_path / ".vnc"
            xstartup = vnc_dir / "xstartup"
            self.run_as(
                username,
                home_dir,
                [
                    "sh",
                    "-c",
                    'mkdir -p "$1" && chmod 0700 "$1" && cat >"$2" '
                    '&& chmod 0755 "$2"',
                    "sh",
                    vnc_dir.as_posix(),
                    xstartup.as_posix(),
                ],
                input_text=self.xstartup_contents(),
            )
        except (OSError, subprocess.CalledProcessError) as err:
            raise BrokerError(
                502, f"Failed to prepare home directory {home_dir}: {err}"
            ) from err

    def ensure_runtime_dir(self, uid, gid):
        """Per-user node-local scratch for sockets and password files.

        Deliberately not $HOME: home directories are typically a shared NFS
        mount here, where a unix socket is not a usable local rendezvous, and
        where a password file would be readable as that uid from every other
        node and would survive a reboot. On tmpfs it is node-local and gone when
        the host restarts.
        """
        user_runtime_dir = self.runtime_dir / str(uid)
        try:
            user_runtime_dir.mkdir(parents=True, exist_ok=True)
            os.chown(user_runtime_dir, uid, gid)
            os.chmod(user_runtime_dir, 0o700)
        except OSError as err:
            raise BrokerError(
                502,
                f"Failed to prepare desktop runtime directory "
                f"{user_runtime_dir}: {err}",
            ) from err
        return user_runtime_dir
