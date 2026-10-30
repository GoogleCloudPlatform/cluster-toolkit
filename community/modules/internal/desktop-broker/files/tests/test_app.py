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

"""Request routing in the broker: what gets a session, what is counted."""

import asyncio
import types

import pytest

pytest.importorskip("aiohttp")

from desktop_broker.app import Broker  # noqa: E402
from desktop_broker.novnc import NoVncFrontend  # noqa: E402


class _Request:
    def __init__(self, path, upgrade=None):
        self.path = path
        self.headers = {"Upgrade": upgrade} if upgrade else {}


class _Frontend(NoVncFrontend):
    def __init__(self):
        self.served = []

    async def handle(self, request, session):
        self.served.append((request.path, session))
        return "response"


class _Sessions:
    def __init__(self):
        self.ensured = 0
        self.opened = 0
        self.closed = 0

    def ensure_sync(self, username):
        self.ensured += 1
        return {"username": username}

    def connection_opened(self, username):
        self.opened += 1

    def connection_closed(self, username):
        self.closed += 1


class _Store:
    def __init__(self):
        self.touched = []

    def user_lock(self, username):
        return asyncio.Lock()

    def touch(self, username):
        self.touched.append(username)


@pytest.fixture
def broker():
    # Assembled by hand: the real constructor builds every component.
    broker = Broker.__new__(Broker)
    broker.frontend = _Frontend()
    broker.sessions = _Sessions()
    broker.store = _Store()

    async def authenticate(request):
        return "alice"

    broker.authenticate = authenticate
    return broker


def handle(broker, request):
    return asyncio.run(broker.handle(request))


@pytest.mark.parametrize("path", ["/", "/vnc.html", "/index.html"])
def test_entry_documents_get_a_session(broker, path):
    handle(broker, _Request(path))
    assert broker.sessions.ensured == 1
    assert broker.frontend.served == [(path, {"username": "alice"})]


@pytest.mark.parametrize("path", ["/app/ui.js", "/app/styles/base.css", "/app/images/icons/novnc.png"])
def test_static_assets_are_served_without_touching_the_session(broker, path):
    # A page load fetches dozens of these in parallel; each must not take the
    # user's lock, rewrite their record and probe the display.
    handle(broker, _Request(path))
    assert broker.sessions.ensured == 0
    assert broker.frontend.served == [(path, None)]


def test_websocket_is_counted_for_its_whole_life(broker):
    seen = {}

    async def relay(request, session):
        seen["open_during"] = broker.sessions.opened - broker.sessions.closed
        return "ws"

    broker.frontend.handle = relay
    assert handle(broker, _Request("/websockify", upgrade="websocket")) == "ws"
    assert broker.sessions.ensured == 1
    assert seen["open_during"] == 1
    assert broker.sessions.opened == broker.sessions.closed == 1
    # Closing is when the record last learns the desktop was in use.
    assert broker.store.touched == ["alice"]


def test_websocket_count_is_released_when_the_relay_fails(broker):
    async def failing_relay(request, session):
        raise OSError("display went away")

    broker.frontend.handle = failing_relay
    response = handle(broker, _Request("/websockify", upgrade="websocket"))
    assert response.status == 500
    assert broker.sessions.opened == broker.sessions.closed == 1
    assert broker.store.touched == ["alice"]


def test_needs_session_ignores_the_query_string():
    frontend = NoVncFrontend(types.SimpleNamespace(novnc_dir=None))
    # The runtime health check is /vnc.html?path=websockify; aiohttp's
    # request.path carries no query, which is what this relies on.
    assert frontend.needs_session(_Request("/vnc.html"))
    assert not frontend.needs_session(_Request("/vnc.html.map"))
