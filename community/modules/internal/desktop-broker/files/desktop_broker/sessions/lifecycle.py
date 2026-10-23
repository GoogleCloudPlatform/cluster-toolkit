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
from ..oslogin import ProfileNotFound

LOG = logging.getLogger("ghpc-desktop-broker")

START_TIMEOUT_SECONDS = 15
CLEANUP_INTERVAL_SECONDS = 300
# Where the X server leaves its per-display lock and socket.
X11_TMP_DIR = Path("/tmp")


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
        # Live relays per user. A desktop with one is in use whatever its
        # record says: the relay itself never touches the record.
        self.active_connections = {}

    # -- live connections --------------------------------------------------

    def connection_opened(self, email):
        key = self.store.key(email)
        self.active_connections[key] = self.active_connections.get(key, 0) + 1

    def connection_closed(self, email):
        key = self.store.key(email)
        remaining = self.active_connections.get(key, 0) - 1
        if remaining > 0:
            self.active_connections[key] = remaining
        else:
            self.active_connections.pop(key, None)

    def has_connections(self, email):
        return self.active_connections.get(self.store.key(email), 0) > 0

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

    def _remove_stale_x11_files(self, display_number):
        """Clear the lock and socket a dead Xvnc leaves in /tmp.

        "vncserver -kill" runs as the session's user and /tmp is sticky, so
        it cannot remove what a previous owner of this display left: after
        an Xvnc crash, or once the slot has passed to another user. Xvnc then
        refuses to start. The broker is root and can.
        """
        for stale_path in (
            X11_TMP_DIR / f".X{display_number}-lock",
            X11_TMP_DIR / ".X11-unix" / f"X{display_number}",
        ):
            try:
                stale_path.unlink()
            except FileNotFoundError:
                pass
            except OSError as err:
                raise BrokerError(
                    502, f"Failed to clear stale X11 file {stale_path}: {err}"
                ) from err

    def _start(self, session):
        self._kill(session)
        self._remove_stale_x11_files(session["display_number"])

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
        try:
            self._remove_stale_x11_files(session["display_number"])
        except BrokerError as err:
            # The record still has to go; _start clears these again anyway.
            LOG.warning("%s", err.message)
        self.store.delete(session["email"])

    # -- ensure ------------------------------------------------------------

    def ensure_sync(self, identity):
        email = identity["email"]
        session = self.store.load(email)
        # Resolved on every hand-off, including a reconnect to a live session,
        # so revoking OS Login access revokes the desktop (see oslogin.py). The
        # directory caches profiles, so this is not a metadata call per request.
        try:
            profile = self.oslogin.resolve(
                identity.get("login_uid"), identity.get("username_hint")
            )
        except ProfileNotFound:
            if session:
                self.stop(session)
            raise
        if session and session.get("username") != profile["username"]:
            # The identity now maps to another POSIX account: never hand over
            # a desktop running as the old one.
            self.stop(session)
            session = None

        if self.is_ready(session):
            session["last_accessed"] = int(time.time())
            self.store.save(session)
            return session

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
        user_session_dir = self.user_environment.ensure_session_dir(
            passwd_entry.pw_uid, passwd_entry.pw_gid
        )
        xstartup = self.user_environment.write_xstartup(
            passwd_entry.pw_name, home_dir, user_session_dir
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
            session["xstartup"] = str(xstartup)
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
            await self.reap_idle(run_blocking, timeout)

    async def reap_idle(self, run_blocking, timeout):
        now = int(time.time())
        # Off the event loop: listing sessions globs the state directory
        # and reads every file in it.
        sessions = await run_blocking(self.store.list)
        for session in sessions:
            email = session.get("email")
            if not email or self.has_connections(email):
                continue
            if now - int(session.get("last_accessed", 0)) < timeout:
                continue
            try:
                async with self.store.user_lock(email):
                    # That record was read before the lock was taken. The user
                    # may have come back while this waited for it.
                    current = await run_blocking(self.store.load, email)
                    if not current or self.has_connections(email):
                        continue
                    idle = int(time.time()) - int(current.get("last_accessed", 0))
                    if idle < timeout:
                        continue
                    await run_blocking(self.stop, current)
            except Exception:
                LOG.exception(
                    "Failed to clean up idle desktop session for %s", email
                )
