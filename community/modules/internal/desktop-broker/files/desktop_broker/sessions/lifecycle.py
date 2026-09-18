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

"""Starting, reusing and reaping one desktop session per user."""

import asyncio
import logging
import socket
import time
from pathlib import Path

from ..errors import BrokerError

LOG = logging.getLogger("ghpc-desktop-broker")

START_TIMEOUT_SECONDS = 15
CLEANUP_INTERVAL_SECONDS = 300


class SessionManager:
    """Owns the lifecycle of per-user desktop sessions.

    Composes the pieces rather than inheriting from them, so each can be tested
    on its own: a store for records, a user environment for privileged work, a
    backend for the VNC flavour, a credential issuer for RFB auth.
    """

    def __init__(
        self,
        config,
        store,
        user_environment,
        oslogin_directory,
        vnc_backend,
        passwd_lookup,
    ):
        self.config = config
        self.store = store
        self.user_environment = user_environment
        self.oslogin = oslogin_directory
        self.vnc_backend = vnc_backend
        self.passwd_lookup = passwd_lookup
        self._cleanup_task = None

    # -- readiness ---------------------------------------------------------

    def _endpoint_ready(self, session):
        return self._socket_ready(session.get("vnc_socket"))

    def _socket_ready(self, socket_path):
        if not socket_path:
            return False
        connection = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        connection.settimeout(0.5)
        try:
            connection.connect(str(socket_path))
            return True
        except OSError:
            return False
        finally:
            connection.close()

    def _wait_until_ready(self, session):
        deadline = time.monotonic() + START_TIMEOUT_SECONDS
        while time.monotonic() < deadline:
            if self._endpoint_ready(session):
                return True
            time.sleep(0.25)
        return False

    def is_ready(self, session):
        return bool(session) and self._endpoint_ready(session)

    # -- start and stop ----------------------------------------------------

    def _kill(self, session):
        self.user_environment.run_as(
            session["username"],
            session["home_dir"],
            self.vnc_backend.kill_command(session["display_number"]),
            check=False,
        )

    def _start(self, session):
        self._kill(session)

        # A crashed Xvnc leaves its socket behind and the next bind fails.
        try:
            Path(session["vnc_socket"]).unlink()
        except FileNotFoundError:
            pass
        except OSError as err:
            raise BrokerError(
                502,
                f"Failed to clear stale desktop socket "
                f"{session['vnc_socket']}: {err}",
            ) from err

        self.user_environment.run_as(
            session["username"],
            session["home_dir"],
            self.vnc_backend.start_command(session),
            check=True,
            environment=self.vnc_backend.environment(),
        )

    def stop(self, session):
        self._kill(session)
        socket_path = session.get("vnc_socket")
        if socket_path:
            try:
                Path(socket_path).unlink()
            except OSError:
                pass
        self.store.delete(session["email"])

    # -- ensure ------------------------------------------------------------

    def ensure_sync(self, identity):
        email = identity["email"]
        session = self.store.load(email)
        if self.is_ready(session):
            session["last_accessed"] = int(time.time())
            self.store.save(session)
            return session

        profile = self.oslogin.resolve(
            identity.get("login_uid"), identity.get("username_hint")
        )
        passwd_entry = self.passwd_lookup(profile["username"])
        home_dir = (
            passwd_entry.pw_dir
            or profile.get("homeDirectory")
            or f"/home/{passwd_entry.pw_name}"
        )
        self.user_environment.ensure_home(
            passwd_entry.pw_name,
            home_dir,
            passwd_entry.pw_uid,
            passwd_entry.pw_gid,
        )
        user_runtime_dir = self.user_environment.ensure_runtime_dir(
            passwd_entry.pw_uid, passwd_entry.pw_gid
        )

        # Reserve the slot and persist the record before starting anything, so
        # two users arriving together cannot claim one display.
        with self.store.allocation_lock:
            existing = self.store.load(email)
            slot = (
                existing["slot"]
                if existing and existing.get("slot") is not None
                else self.store.allocate_slot(email)
            )
            display_number = self.config.display_number(slot)
            session = {
                "email": email,
                "slot": int(slot),
                "username": passwd_entry.pw_name,
                "uid": passwd_entry.pw_uid,
                "gid": passwd_entry.pw_gid,
                "home_dir": home_dir,
                "display_number": display_number,
                "last_accessed": int(time.time()),
            }
            session["vnc_socket"] = str(user_runtime_dir / "vnc.sock")
            self.store.save(session)

        if not self._endpoint_ready(session):
            self._start(session)

        if not self._wait_until_ready(session):
            raise BrokerError(
                502, "The desktop session failed to start correctly."
            )

        session["last_accessed"] = int(time.time())
        self.store.save(session)
        return session

    # -- background reaping ------------------------------------------------

    async def start_cleanup(self, run_blocking):
        self._cleanup_task = asyncio.create_task(self._cleanup_loop(run_blocking))

    async def stop_cleanup(self):
        if self._cleanup_task is not None:
            self._cleanup_task.cancel()
            try:
                await self._cleanup_task
            except asyncio.CancelledError:
                pass

    async def _cleanup_loop(self, run_blocking):
        timeout = self.config.session_idle_timeout_seconds
        if timeout <= 0:
            return
        while True:
            await asyncio.sleep(CLEANUP_INTERVAL_SECONDS)
            now = int(time.time())
            # Off the event loop: listing sessions globs the state directory
            # and reads every file in it.
            sessions = await run_blocking(self.store.list)
            for session in sessions:
                if now - int(session.get("last_accessed", 0)) < timeout:
                    continue
                try:
                    async with self.store.user_lock(session["email"]):
                        await run_blocking(self.stop, session)
                except Exception:
                    LOG.exception(
                        "Failed to clean up idle desktop session for %s",
                        session.get("email"),
                    )
