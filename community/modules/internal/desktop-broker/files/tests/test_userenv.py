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

"""The user's session environment."""

import os
import stat

import pytest

from desktop_broker.sessions.userenv import UserEnvironment


class _Backend:
    def session_command(self):
        return ["dbus-launch", "startxfce4"]

    def session_environment(self):
        return {"VGL_DISPLAY": "egl0"}


@pytest.fixture
def env(tmp_path):
    return UserEnvironment(tmp_path / "run", _Backend())


@pytest.fixture
def xstartup(env):
    return env.xstartup_contents()


def test_runtime_dir_is_traversable(env):
    # Parent of the per-user 0700 directories, so it must stay traversable.
    assert stat.S_IMODE(os.stat(env.runtime_dir).st_mode) == 0o755


def test_per_user_runtime_dir_is_private(env, tmp_path):
    created = env.ensure_runtime_dir(os.getuid(), os.getgid())
    assert stat.S_IMODE(os.stat(created).st_mode) == 0o700
    assert created.parent == env.runtime_dir


def test_xstartup_sources_the_login_profile(xstartup):
    # Without this a login-node desktop has no sbatch and no environment
    # modules: runuser is a non-login invocation and inherits neither.
    assert ". /etc/profile" in xstartup


def test_xstartup_is_non_fatal_about_the_profile(xstartup):
    # A profile script exiting non-zero must not stop the desktop starting.
    assert "|| true" in xstartup
    assert "[ -r /etc/profile ]" in xstartup


def test_xstartup_exports_backend_environment(xstartup):
    assert "export VGL_DISPLAY=egl0" in xstartup


def test_xstartup_execs_the_session_last(xstartup):
    assert xstartup.rstrip().endswith("exec dbus-launch startxfce4")


def test_xstartup_clears_inherited_session_bus(xstartup):
    assert "unset DBUS_SESSION_BUS_ADDRESS" in xstartup
    assert "unset SESSION_MANAGER" in xstartup


def test_xstartup_quotes_session_arguments(tmp_path):
    class Awkward(_Backend):
        def session_command(self):
            return ["run", "a b; rm -rf /"]

        def session_environment(self):
            return {"K": "v; rm -rf /"}

    contents = UserEnvironment(tmp_path / "run2", Awkward()).xstartup_contents()
    # Neither the argv nor the environment may break out of its shell word.
    assert "'a b; rm -rf /'" in contents
    assert "export K='v; rm -rf /'" in contents


def test_run_as_builds_a_runuser_invocation(env, monkeypatch):
    captured = {}

    def fake_run(command, **kwargs):
        captured["command"] = command
        captured["kwargs"] = kwargs
        return object()

    monkeypatch.setattr("desktop_broker.sessions.userenv.subprocess.run", fake_run)
    env.run_as("alice", "/home/alice", ["id"], environment={"X": "1"})

    command = captured["command"]
    assert command[:4] == ["runuser", "-u", "alice", "--"]
    assert "HOME=/home/alice" in command
    assert "USER=alice" in command
    assert "X=1" in command
    assert command[-1] == "id"
