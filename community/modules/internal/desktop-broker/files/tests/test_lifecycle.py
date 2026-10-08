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
import time
from collections import namedtuple

import pytest

from desktop_broker.sessions import lifecycle
from desktop_broker.sessions.lifecycle import SessionManager
from desktop_broker.sessions.store import SessionStore

Passwd = namedtuple("Passwd", "pw_name pw_uid pw_gid pw_dir")
ALICE = Passwd("alice", 1001, 1001, "/home/alice")
TIMEOUT = 3600


class _Config:
    session_idle_timeout_seconds = TIMEOUT

    @staticmethod
    def display_number(slot):
        return 1 + slot


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

    def write_xstartup(self, username, home_dir, user_runtime_dir):
        return user_runtime_dir / "xstartup"

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
    @staticmethod
    def resolve(login_uid, username_hint=None):
        return {"username": "alice"}


async def run_blocking(func, *args):
    return func(*args)


@pytest.fixture
def manager(tmp_path, monkeypatch):
    manager = SessionManager(
        config=_Config(),
        store=SessionStore(tmp_path / "state", max_user_sessions=4),
        user_environment=_UserEnvironment(tmp_path / "run"),
        oslogin_directory=_OsLogin(),
        vnc_backend=_Backend(),
        passwd_lookup=lambda username: ALICE,
    )
    # No real display: the socket counts as ready once a start was attempted.
    monkeypatch.setattr(manager, "_endpoint_ready", lambda session: started(manager))
    return manager


def started(manager):
    return any(cmd[:1] == ["vncserver"] and cmd[1:2] != ["-kill"]
               for cmd in manager.user_environment.commands)


def session_for(manager, email="alice@example.com", age=0):
    record = {
        "email": email,
        "slot": 0,
        "username": "alice",
        "uid": 1001,
        "gid": 1001,
        "home_dir": "/home/alice",
        "display_number": 1,
        "vnc_socket": str(manager.user_environment.runtime_dir / "1001" / "vnc.sock"),
        "xstartup": str(manager.user_environment.runtime_dir / "1001" / "xstartup"),
        "last_accessed": int(time.time()) - age,
    }
    manager.store.save(record)
    return record


def reap(manager):
    asyncio.run(manager.reap_idle(run_blocking, TIMEOUT))


# -- idle reaping -----------------------------------------------------------


def test_idle_session_is_reaped(manager):
    session_for(manager, age=TIMEOUT + 1)
    reap(manager)
    assert manager.store.load("alice@example.com") is None


def test_recent_session_is_kept(manager):
    session_for(manager, age=TIMEOUT - 60)
    reap(manager)
    assert manager.store.load("alice@example.com") is not None


def test_session_with_a_live_relay_is_never_reaped(manager):
    # The relay never touches the record, so by age alone this looks idle.
    session_for(manager, age=TIMEOUT * 10)
    manager.connection_opened("Alice@Example.com")
    reap(manager)
    assert manager.store.load("alice@example.com") is not None


def test_reaper_rechecks_the_record_under_the_lock(manager):
    session_for(manager, age=TIMEOUT + 1)

    # The user reconnects while the reaper is waiting for their lock: the
    # listing it holds is stale, and the fresh record must win.
    async def reconnect_during(func, *args):
        if func == manager.store.load:
            manager.store.touch(args[0])
        return func(*args)

    asyncio.run(manager.reap_idle(reconnect_during, TIMEOUT))
    assert manager.store.load("alice@example.com") is not None


def test_connection_count_is_balanced(manager):
    manager.connection_opened("alice@example.com")
    manager.connection_opened("ALICE@example.com")
    manager.connection_closed("alice@example.com")
    assert manager.has_connections("alice@example.com")
    manager.connection_closed("alice@example.com")
    assert not manager.has_connections("alice@example.com")
    # Never negative, so one spurious close cannot mask a later open.
    manager.connection_closed("alice@example.com")
    manager.connection_opened("alice@example.com")
    assert manager.has_connections("alice@example.com")


# -- stale X11 files ----------------------------------------------------------


@pytest.fixture
def x11_tmp(tmp_path, monkeypatch):
    tmp = tmp_path / "x11tmp"
    (tmp / ".X11-unix").mkdir(parents=True)
    monkeypatch.setattr(lifecycle, "X11_TMP_DIR", tmp)
    return tmp


def leave_stale_files(x11_tmp, display):
    lock = x11_tmp / f".X{display}-lock"
    sock = x11_tmp / ".X11-unix" / f"X{display}"
    lock.write_text("12345\n")
    sock.write_text("")
    return lock, sock


def test_start_clears_what_a_previous_owner_left(manager, x11_tmp):
    # "vncserver -kill" as the new user cannot remove these in sticky /tmp.
    lock, sock = leave_stale_files(x11_tmp, 1)
    manager._start(session_for(manager))
    assert not lock.exists()
    assert not sock.exists()
    assert started(manager)


def test_start_tolerates_absent_x11_files(manager, x11_tmp):
    manager._start(session_for(manager))
    assert started(manager)


def test_stop_clears_x11_files_and_the_record(manager, x11_tmp):
    lock, sock = leave_stale_files(x11_tmp, 1)
    manager.stop(session_for(manager))
    assert not lock.exists()
    assert not sock.exists()
    assert manager.store.load("alice@example.com") is None


# -- xstartup -----------------------------------------------------------------


def test_session_records_its_node_local_xstartup(manager, x11_tmp):
    session = manager.ensure_sync({"email": "alice@example.com", "login_uid": "1"})
    expected = manager.user_environment.runtime_dir / "1001" / "xstartup"
    assert session["xstartup"] == str(expected)
    assert manager.store.load("alice@example.com")["xstartup"] == str(expected)
