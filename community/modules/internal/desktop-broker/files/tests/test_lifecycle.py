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

"""Starting, stopping and reaping sessions, with every privileged piece faked."""

import asyncio
import os
import socket
import subprocess
import tempfile
import time
from collections import namedtuple
from pathlib import Path

import pytest

from desktop_broker.errors import BrokerError
from desktop_broker.oslogin import ProfileNotFound
from desktop_broker.sessions import lifecycle
from desktop_broker.sessions.lifecycle import SessionManager
from desktop_broker.sessions.store import SessionStore

Passwd = namedtuple("Passwd", "pw_name pw_uid pw_gid pw_dir")
ALICE = Passwd("alice", 1001, 1001, "/home/alice")
BOB = Passwd("bob", 1002, 1002, "/home/bob")
USERS = {"alice": ALICE, "bob": BOB}
TIMEOUT = 3600


class _Config:
    session_idle_timeout_seconds = TIMEOUT

    @staticmethod
    def display_number(slot):
        return 1 + slot

    @staticmethod
    def display_candidates(slot):
        return [1 + slot] + SPARES


SPARES = [10, 11]


class _UserEnvironment:
    def __init__(self, runtime_dir):
        self.runtime_dir = runtime_dir
        self.commands = []

    def ensure_home(self, username, home_dir, uid, gid):
        pass

    def ensure_runtime_dir(self, uid, gid):
        path = self.runtime_dir / str(uid)
        path.mkdir(parents=True, exist_ok=True)
        return path

    def ensure_session_dir(self, uid, gid):
        path = self.runtime_dir.parent / "sessions" / str(uid)
        path.mkdir(parents=True, exist_ok=True)
        return path

    def write_xstartup(self, username, home_dir, user_session_dir):
        return user_session_dir / "xstartup"

    def run_as(self, username, home_dir, args, **kwargs):
        self.commands.append(list(args))


class _Backend:
    def kill_command(self, display_number):
        return ["vncserver", "-kill", f":{display_number}"]

    def start_command(self, session):
        return ["vncserver", f":{session['display_number']}"]

    def environment(self):
        return {}


class _OsLogin:
    """Knows every account it is asked about unless a test sets an error."""

    def __init__(self):
        self.error = None
        self.asked = []

    def resolve(self, username):
        self.asked.append(username)
        if self.error:
            raise self.error
        return {"username": username}


async def run_blocking(func, *args):
    return func(*args)


@pytest.fixture(autouse=True)
def x11_tmp(monkeypatch):
    # Never the real /tmp, and short: a unix socket path is limited to about
    # 100 bytes, which tmp_path can exceed.
    with tempfile.TemporaryDirectory(dir="/tmp") as short:
        tmp = Path(short)
        (tmp / ".X11-unix").mkdir()
        monkeypatch.setattr(lifecycle, "X11_TMP_DIR", tmp)
        yield tmp


@pytest.fixture
def manager(tmp_path, monkeypatch):
    manager = SessionManager(
        config=_Config(),
        store=SessionStore(tmp_path / "state", max_user_sessions=4),
        user_environment=_UserEnvironment(tmp_path / "run"),
        oslogin_directory=_OsLogin(),
        vnc_backend=_Backend(),
        passwd_lookup=lambda username: USERS[username],
    )
    # No real display: the socket counts as ready once a start was attempted.
    monkeypatch.setattr(manager, "_endpoint_ready", lambda session: bool(started_on(manager)))
    return manager


def started_on(manager):
    return [cmd[1] for cmd in manager.user_environment.commands
            if cmd[:1] == ["vncserver"] and cmd[1:2] != ["-kill"]]


def killed(manager):
    return any(cmd[1:2] == ["-kill"] for cmd in manager.user_environment.commands)


def session_for(manager, username="alice", age=0):
    record = {
        "slot": 0,
        "username": username,
        "uid": 1001,
        "gid": 1001,
        "home_dir": "/home/alice",
        "display_number": 1,
        "vnc_socket": str(manager.user_environment.runtime_dir / "1001" / "vnc.sock"),
        "xstartup": str(manager.user_environment.runtime_dir.parent / "sessions" / "1001" / "xstartup"),
        "last_accessed": int(time.time()) - age,
    }
    manager.store.save(record)
    return record


def reap(manager, timeout=TIMEOUT):
    asyncio.run(manager.reap_idle(run_blocking, timeout))


# -- reusing a session ----------------------------------------------------------


def test_live_session_is_reused_while_still_authorised(manager, monkeypatch):
    session_for(manager)
    monkeypatch.setattr(manager, "_endpoint_ready", lambda session: True)
    assert manager.ensure_sync("alice")["username"] == "alice"
    assert manager.oslogin.asked == ["alice"]
    assert not started_on(manager) and not killed(manager)


@pytest.mark.parametrize(
    "error, stopped",
    [(ProfileNotFound(403, "revoked"), True), (BrokerError(502, "metadata down"), False)],
    ids=["revoked-stops-it", "lookup-failure-keeps-it"],
)
def test_a_refused_lookup_fails_and_only_revocation_stops_the_session(manager, monkeypatch, error, stopped):
    session_for(manager)
    monkeypatch.setattr(manager, "_endpoint_ready", lambda session: True)
    manager.oslogin.error = error
    with pytest.raises(BrokerError) as excinfo:
        manager.ensure_sync("alice")
    assert excinfo.value.status == error.status
    assert killed(manager) is stopped
    assert (manager.store.load("alice") is None) is stopped


def test_new_session_records_its_node_local_xstartup(manager):
    session = manager.ensure_sync("alice")
    expected = manager.user_environment.runtime_dir.parent / "sessions" / "1001" / "xstartup"
    assert session["xstartup"] == str(expected)
    assert manager.store.load("alice")["xstartup"] == str(expected)


# -- reaping ------------------------------------------------------------------


@pytest.mark.parametrize("age, kept", [(TIMEOUT + 1, False), (TIMEOUT - 60, True)])
def test_only_idle_sessions_are_reaped(manager, age, kept):
    session_for(manager, age=age)
    reap(manager)
    assert (manager.store.load("alice") is not None) is kept


def test_session_with_a_live_relay_is_never_reaped(manager):
    # The relay never touches the record, so by age alone this looks idle.
    session_for(manager, age=TIMEOUT * 10)
    manager.connection_opened("alice")
    reap(manager)
    assert manager.store.load("alice") is not None


def test_connection_count_never_goes_negative(manager):
    manager.connection_opened("alice")
    manager.connection_closed("alice")
    assert not manager.has_connections("alice")
    # A spurious close must not mask the next open.
    manager.connection_closed("alice")
    manager.connection_opened("alice")
    assert manager.has_connections("alice")


def test_reaper_rechecks_the_record_under_the_lock(manager):
    session_for(manager, age=TIMEOUT + 1)

    # The user reconnects while the reaper is waiting for their lock: the
    # listing it holds is stale, and the fresh record must win.
    async def reconnect_during(func, *args):
        if func == manager.store.load:
            manager.store.touch(args[0])
        return func(*args)

    asyncio.run(manager.reap_idle(reconnect_during, TIMEOUT))
    assert manager.store.load("alice") is not None


@pytest.mark.parametrize("ready", [True, False], ids=["live-kept", "dead-freed"])
def test_no_timeout_still_frees_only_dead_sessions(manager, monkeypatch, ready):
    session_for(manager, age=TIMEOUT * 10)
    monkeypatch.setattr(manager, "_endpoint_ready", lambda session: ready)
    reap(manager, timeout=0)
    assert (manager.store.load("alice") is not None) is ready


def test_cleanup_loop_survives_a_failed_pass(manager, monkeypatch):
    monkeypatch.setattr(lifecycle, "CLEANUP_INTERVAL_SECONDS", 0)
    passes = []

    async def flaky(run_blocking, timeout):
        passes.append(timeout)
        if len(passes) == 1:
            raise OSError("state directory unreadable")
        if len(passes) == 3:
            raise asyncio.CancelledError

    monkeypatch.setattr(manager, "reap_idle", flaky)
    with pytest.raises(asyncio.CancelledError):
        asyncio.run(manager._cleanup_loop(run_blocking))
    assert len(passes) == 3


# -- X11 files left in /tmp -------------------------------------------------------


def dead_pid():
    child = subprocess.Popen(["true"])
    child.wait()
    return child.pid


def leave_x11_files(x11_tmp, display, pid):
    lock = x11_tmp / f".X{display}-lock"
    sock = x11_tmp / ".X11-unix" / f"X{display}"
    # Padded as Xorg writes it.
    lock.write_text(f"{pid:>10}\n")
    sock.write_text("")
    return lock, sock


@pytest.mark.parametrize("contents", ["dead-pid", "not a pid"])
def test_start_clears_what_a_previous_owner_left(manager, x11_tmp, contents):
    # "vncserver -kill" as the new user cannot remove these in sticky /tmp.
    lock, sock = leave_x11_files(x11_tmp, 1, dead_pid())
    if contents != "dead-pid":
        lock.write_text(f"{contents}\n")
    manager._start(session_for(manager))
    assert not lock.exists() and not sock.exists()
    assert started_on(manager) == [":1"]


def test_start_moves_off_a_live_x_server_and_back_once_free(manager, x11_tmp):
    # A user's own vncserver on :1 of a shared login node.
    lock, _ = leave_x11_files(x11_tmp, 1, os.getpid())
    session = session_for(manager)
    manager._start(session)
    assert lock.exists()
    assert manager.store.load("alice")["display_number"] == 10

    lock.unlink()
    manager._start(session)
    assert started_on(manager) == [":10", ":1"]
    assert session["display_number"] == 1


@pytest.mark.parametrize("listening", [True, False])
def test_an_x11_socket_holds_the_display_only_while_listening(manager, x11_tmp, listening):
    path = x11_tmp / ".X11-unix" / "X1"
    listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    listener.bind(str(path))
    if listening:
        listener.listen(1)
    else:
        listener.close()
    try:
        manager._start(session_for(manager))
    finally:
        listener.close()
    assert path.exists() is listening
    assert started_on(manager) == [":10" if listening else ":1"]


def test_a_directory_at_the_lock_is_never_removed(manager, x11_tmp):
    planted = x11_tmp / ".X1-lock"
    planted.mkdir()
    (planted / "keep").write_text("")
    manager._start(session_for(manager))
    assert (planted / "keep").exists()
    assert started_on(manager) == [":10"]


def test_a_symlink_at_the_lock_is_removed_not_followed(manager, x11_tmp, tmp_path):
    target = tmp_path / "victim"
    target.write_text(f"{os.getpid()}\n")
    (x11_tmp / ".X1-lock").symlink_to(target)
    manager._start(session_for(manager))
    assert not (x11_tmp / ".X1-lock").is_symlink()
    assert target.read_text() == f"{os.getpid()}\n"
    assert started_on(manager) == [":1"]


def test_a_display_held_by_another_session_is_skipped(manager, x11_tmp):
    leave_x11_files(x11_tmp, 1, os.getpid())
    other = session_for(manager, username="bob")
    other.update(slot=1, display_number=10)
    manager.store.save(other)
    manager._start(session_for(manager))
    assert started_on(manager) == [":11"]


def test_no_free_display_is_a_clear_refusal(manager, x11_tmp):
    for display in [1] + SPARES:
        leave_x11_files(x11_tmp, display, os.getpid())
    with pytest.raises(BrokerError) as excinfo:
        manager._start(session_for(manager))
    assert excinfo.value.status == 503
    assert not started_on(manager)


@pytest.mark.parametrize("live", [True, False])
def test_stop_clears_only_stale_x11_files(manager, x11_tmp, live):
    lock, _ = leave_x11_files(x11_tmp, 1, os.getpid() if live else dead_pid())
    manager.stop(session_for(manager))
    assert lock.exists() is live
    assert manager.store.load("alice") is None
