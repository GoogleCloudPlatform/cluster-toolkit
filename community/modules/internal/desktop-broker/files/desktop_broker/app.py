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

"""Wiring the pieces together into an aiohttp application.

Deliberately the only place that knows about every component, so the components
themselves stay independently testable.
"""

import asyncio
import logging

from aiohttp import web

from . import oslogin
from .backends import registry as backends
from .errors import BrokerError
from .identity import resolver as identity
from .novnc import NoVncFrontend
from .sessions.lifecycle import SessionManager
from .sessions.store import SessionStore
from .sessions.userenv import UserEnvironment

LOG = logging.getLogger("ghpc-desktop-broker")


async def run_blocking(func, *args):
    """Run a blocking call off the event loop."""
    to_thread = getattr(asyncio, "to_thread", None)
    if to_thread is not None:
        return await to_thread(func, *args)
    loop = asyncio.get_running_loop()
    return await loop.run_in_executor(None, lambda: func(*args))


class Broker:
    """Request handling: authenticate, ensure a session, hand off."""

    def __init__(self, config):
        self.config = config

        config.log_dir.mkdir(parents=True, exist_ok=True)

        self.vnc_backend = backends.create(
            config.vnc_backend,
            config.session_resolution,
            gpu_acceleration=config.gpu_acceleration,
        )
        self.identity = identity.Resolver(config)
        self.store = SessionStore(config.state_dir, config.max_user_sessions)
        self.user_environment = UserEnvironment(
            config.runtime_dir, self.vnc_backend
        )
        self.sessions = SessionManager(
            config=config,
            store=self.store,
            user_environment=self.user_environment,
            oslogin_directory=oslogin.Directory(),
            vnc_backend=self.vnc_backend,
            passwd_lookup=oslogin.lookup_passwd,
        )
        self.frontend = NoVncFrontend(config)

        self._report_gpu_posture()

    def _report_gpu_posture(self):
        # Say so when requested GPU acceleration will not happen. Falling back
        # to software rendering is the right behaviour, but it is silent, and a
        # software desktop is easily mistaken for a slow one.
        reason = self.vnc_backend.acceleration_diagnostic()
        if reason:
            LOG.warning(
                "enable_gpu_acceleration is set but sessions will use SOFTWARE "
                "rendering: %s",
                reason,
            )
        elif self.vnc_backend.accelerated():
            LOG.info(
                "Hardware OpenGL enabled for desktop sessions (backend=%s).",
                self.vnc_backend.name,
            )

    async def start(self):
        await self.sessions.start_cleanup(run_blocking)

    async def stop(self):
        await self.sessions.stop_cleanup()

    async def authenticate(self, request):
        presented = identity.presented_from_headers(request.headers)
        # Verification may fetch signing certificates, so keep it off the loop.
        return await run_blocking(self.identity.resolve, presented)

    async def ensure_session(self, resolved):
        async with self.store.user_lock(resolved["email"]):
            return await run_blocking(self.sessions.ensure_sync, resolved)

    async def handle_health(self, request):
        # Deliberately unauthenticated and free of detail: a fronting proxy
        # needs to know the broker is up without holding the proxy secret.
        return web.Response(text="ok\n")

    async def handle(self, request):
        try:
            resolved = await self.authenticate(request)
            session = await self.ensure_session(resolved)
            return await self.frontend.handle(request, session)
        except BrokerError as err:
            return web.Response(status=err.status, text=f"{err.message}\n")
        except Exception:
            LOG.exception("Unhandled desktop broker failure")
            return web.Response(
                status=500,
                text="The desktop broker encountered an unexpected error.\n",
            )


def build(config):
    broker = Broker(config)
    app = web.Application()
    app["desktop_broker"] = broker
    app.router.add_get("/healthz", broker.handle_health)
    app.router.add_route("*", "/{path:.*}", broker.handle)

    async def on_startup(app):
        await app["desktop_broker"].start()

    async def on_cleanup(app):
        await app["desktop_broker"].stop()

    app.on_startup.append(on_startup)
    app.on_cleanup.append(on_cleanup)
    return app
