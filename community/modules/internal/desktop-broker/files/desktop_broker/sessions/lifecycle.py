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
import os
import socket
import stat
import time
from pathlib import Path

from ..errors import BrokerError
from ..oslogin import ProfileNotFound

LOG = logging.getLogger("ghpc-desktop-broker")

START_TIMEOUT_SECONDS = 15
CLEANUP_INTERVAL_SECONDS = 300
# Where the X server leaves its per-display lock and socket.
X11_TMP_DIR = Path("/tmp")
X11_ABSENT, X11_STALE, X11_LIVE = "absent", "stale", "live"


def _read_lock_pid(path):
    """The PID an X lock file names, or 0 if it names none.

    Opened without following a symlink or blocking on a FIFO: /tmp is
    world-writable, and the broker reads this as root.
    """
    try:
        descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    except OSError:
        return 0
    try:
        if not stat.S_ISREG(os.fstat(descriptor).st_mode):
            return 0
        return int(os.read(descriptor, 32).decode("ascii", "replace").strip())
    except (OSError, ValueError):
        return 0
    finally:
        os.close(descriptor)


def _pid_alive(pid):
    if pid <= 0:
        return False
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


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

    def connection_opened(self, username):
        self.active_connections[username] = (
            self.active_connections.get(username, 0) + 1
        )

    def connection_closed(self, username):
        remaining = self.active_connections.get(username, 0) - 1
        if remaining > 0:
            self.active_connections[username] = remaining
        else:
            self.active_connections.pop(username, None)

    def has_connections(self, username):
        return self.active_connections.get(username, 0) > 0

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

    def _x11_file_state(self, path):
        try:
            mode = os.lstat(path).st_mode
        except FileNotFoundError:
            return X11_ABSENT
        if stat.S_ISDIR(mode):
            # Nothing an X server leaves, and not safe to remove as root in a
            # world-writable directory. Treated as occupying the display.
            return X11_LIVE
        if stat.S_ISSOCK(mode):
            return X11_LIVE if self._socket_ready(path) else X11_STALE
        if stat.S_ISREG(mode):
            return X11_LIVE if _pid_alive(_read_lock_pid(path)) else X11_STALE
        # A symlink or anything else: unlink() removes the entry itself and
        # never follows it.
        return X11_STALE

    def _clear_display(self, display_number):
        """Free a display of what a dead X server left, if nothing holds it.

        "vncserver -kill" runs as the session's user and /tmp is sticky, so it
        cannot remove what a previous owner of this display left: after an
        Xvnc crash, or once the slot has passed to another user. The broker is
        root and can, but on a shared login node the display may equally be a
        user's own live vncserver or Xvfb, so a file goes only once the process
        or socket behind it is shown to be dead.

        Returns whether the display is free to start on.
        """
        paths = (
            X11_TMP_DIR / f".X{display_number}-lock",
            X11_TMP_DIR / ".X11-unix" / f"X{display_number}",
        )
        states = [self._x11_file_state(path) for path in paths]
        if X11_LIVE in states:
            return False
        for path, state in zip(paths, states):
            if state != X11_STALE:
                continue
            try:
                path.unlink()
            except FileNotFoundError:
                pass
            except OSError as err:
                LOG.warning("Cannot clear stale X11 file %s: %s", path, err)
                return False
        return True

    def _claim_display(self, session):
        """Settle the session on a free display, preferring its slot's own."""
        with self.store.allocation_lock:
            taken = {
                int(other["display_number"])
                for other in self.store.list()
                if other.get("username") != session["username"]
                and other.get("display_number") is not None
            }
            for display_number in self.config.display_candidates(session["slot"]):
                if display_number in taken or not self._clear_display(display_number):
                    continue
                if display_number != self.config.display_number(session["slot"]):
                    LOG.warning(
                        "Display :%d is held by a process the broker did not "
                        "start; using :%d for %s instead.",
                        self.config.display_number(session["slot"]),
                        display_number,
                        session["username"],
                    )
                session["display_number"] = display_number
                self.store.save(session)
                return
        raise BrokerError(
            503,
            "No free X display is available for a desktop session on this "
            "host. Ask an administrator to check for stray X servers.",
        )

    def _start(self, session):
        self._kill(session)
        self._claim_display(session)

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
        # Only what is shown to be dead goes; _start checks again anyway.
        self._clear_display(session["display_number"])
        self.store.delete(session["username"])

    # -- ensure ------------------------------------------------------------

    def ensure_sync(self, username):
        session = self.store.load(username)
        # Checked on every hand-off, including a reconnect to a live session,
        # so revoking OS Login access revokes the desktop (see oslogin.py), and
        # a local account OS Login does not manage never gets one. The
        # directory caches profiles, so this is not a metadata call per request.
        try:
            profile = self.oslogin.resolve(username)
        except ProfileNotFound:
            if session:
                self.stop(session)
            raise

        if self.is_ready(session):
            session["last_accessed"] = int(time.time())
            self.store.save(session)
            return session

        passwd_entry = self.passwd_lookup(username)
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
            existing = self.store.load(username)
            slot = (
                existing["slot"]
                if existing and existing.get("slot") is not None
                else self.store.allocate_slot(username)
            )
            display_number = self.config.display_number(slot)
            session = {
                "slot": int(slot),
                "username": username,
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
        # Runs even with no idle timeout: a session whose desktop has died
        # still holds a slot, and records survive a reboot, so without this a
        # host would run out of slots for good.
        timeout = self.config.session_idle_timeout_seconds
        while True:
            await asyncio.sleep(CLEANUP_INTERVAL_SECONDS)
            try:
                await self.reap_idle(run_blocking, timeout)
            except Exception:
                # An escaped error would end this task, and with it all reaping
                # until the broker restarts.
                LOG.exception("Desktop session cleanup pass failed")

    async def _reapable(self, run_blocking, session, timeout):
        """Idle past the timeout or, with no timeout, desktop gone."""
        if timeout > 0:
            idle = int(time.time()) - int(session.get("last_accessed", 0))
            return idle >= timeout
        return not await run_blocking(self.is_ready, session)

    async def reap_idle(self, run_blocking, timeout):
        # Off the event loop: listing sessions globs the state directory
        # and reads every file in it.
        sessions = await run_blocking(self.store.list)
        for session in sessions:
            username = session.get("username")
            if not username or self.has_connections(username):
                continue
            try:
                if not await self._reapable(run_blocking, session, timeout):
                    continue
                async with self.store.user_lock(username):
                    # That record was read before the lock was taken. The user
                    # may have come back while this waited for it.
                    current = await run_blocking(self.store.load, username)
                    if not current or self.has_connections(username):
                        continue
                    if not await self._reapable(run_blocking, current, timeout):
                        continue
                    await run_blocking(self.stop, current)
            except Exception:
                LOG.exception(
                    "Failed to clean up idle desktop session for %s", username
                )
