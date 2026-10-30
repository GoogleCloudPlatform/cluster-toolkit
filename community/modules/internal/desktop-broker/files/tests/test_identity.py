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

"""Identity from peer credentials.

What is tested is that the answer comes only from the kernel's report of the
connecting process, and which accounts are refused.
"""

import os
import pwd
import socket
import struct
import tempfile
from pathlib import Path

import pytest

from desktop_broker import identity
from desktop_broker.errors import BrokerError


class _Socket:
    def __init__(self, uid=None, error=None):
        self.uid = uid
        self.error = error

    def getsockopt(self, level, option, size):
        if self.error:
            raise self.error
        return struct.pack("iII", 4242, self.uid, self.uid)


class _Transport:
    def __init__(self, connection):
        self.connection = connection

    def get_extra_info(self, name):
        return self.connection if name == "socket" else None


def bind_socket(path):
    # main imports aiohttp, which the repository-wide pytest run lacks.
    return pytest.importorskip("desktop_broker.main").bind_socket(path)


@pytest.fixture
def peercred(monkeypatch):
    # SO_PEERCRED is Linux-only; give other platforms a value so the parsing
    # is still exercised.
    if not hasattr(socket, "SO_PEERCRED"):
        monkeypatch.setattr(socket, "SO_PEERCRED", 17, raising=False)


# 3455320136 is a real OS Login service-account uid. Read as a signed int it
# comes out negative, and the passwd lookup fails for an account that exists.
@pytest.mark.parametrize("uid", [1001, 3455320136])
def test_uid_comes_from_the_kernel_unsigned(peercred, uid):
    assert identity.peer_uid(_Transport(_Socket(uid=uid))) == uid


@pytest.mark.parametrize(
    "transport",
    [None, _Transport(None), _Transport(_Socket(error=OSError("not a unix socket")))],
    ids=["no-transport", "no-socket", "getsockopt-fails"],
)
def test_no_peer_credentials_is_a_refusal(peercred, transport):
    with pytest.raises(BrokerError) as excinfo:
        identity.peer_uid(transport)
    assert excinfo.value.status == 403


def test_root_is_refused():
    with pytest.raises(BrokerError, match="root") as excinfo:
        identity.resolve(0)
    assert excinfo.value.status == 403


def test_a_uid_with_no_account_is_refused(monkeypatch):
    def missing(uid):
        raise KeyError(uid)

    monkeypatch.setattr(pwd, "getpwuid", missing)
    with pytest.raises(BrokerError) as excinfo:
        identity.resolve(1001)
    assert excinfo.value.status == 403


def test_the_account_name_is_returned(monkeypatch):
    monkeypatch.setattr(
        pwd, "getpwuid", lambda uid: pwd.struct_passwd(
            ("alice", "x", uid, uid, "", "/home/alice", "/bin/bash")
        )
    )
    assert identity.resolve(1001) == "alice"


def test_the_socket_is_open_to_every_local_user():
    # Short path: a unix socket path is limited to about 100 bytes.
    with tempfile.TemporaryDirectory(dir="/tmp") as short:
        path = Path(short) / "run" / "broker.sock"
        path.parent.mkdir()
        path.write_text("left by a previous run")
        listener = bind_socket(path)
        try:
            assert path.is_socket()
            assert os.stat(path).st_mode & 0o777 == 0o666
        finally:
            listener.close()


@pytest.mark.skipif(not hasattr(socket, "SO_PEERCRED"), reason="Linux only")
def test_a_real_connection_reports_this_process():
    with tempfile.TemporaryDirectory(dir="/tmp") as short:
        listener = bind_socket(Path(short) / "broker.sock")
        listener.listen(1)
        client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        client.connect(str(Path(short) / "broker.sock"))
        server, _ = listener.accept()
        try:
            assert identity.peer_uid(_Transport(server)) == os.getuid()
        finally:
            for connection in (server, client, listener):
                connection.close()
