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

"""Serving noVNC and relaying RFB over a websocket.

This front end is the transport: the browser talks websocket to the broker and
the broker talks RFB to the display's 0700 socket. Nothing else can reach the
display, so authentication happens once, before the relay starts.
"""

import asyncio
import logging

from aiohttp import WSMsgType, web

from .errors import BrokerError

LOG = logging.getLogger("ghpc-desktop-broker")

RELAY_CHUNK_BYTES = 65536


class NoVncFrontend:
    name = "novnc"
    healthcheck_path = "/vnc.html?path=websockify"

    def __init__(self, config):
        self.novnc_dir = config.novnc_dir

    def _resolve_static_file(self, path):
        candidate = (self.novnc_dir / path.lstrip("/")).resolve()
        try:
            candidate.relative_to(self.novnc_dir)
        except ValueError:
            # Path traversal out of the served directory.
            raise BrokerError(404, "Not found.")
        if candidate.is_dir():
            candidate = candidate / "vnc.html"
        if not candidate.is_file():
            raise BrokerError(404, "Not found.")
        return candidate

    async def handle(self, request, session):
        if request.headers.get("Upgrade", "").lower() == "websocket":
            return await self._bridge(request, session)
        path = request.match_info.get("path", "") or "vnc.html"
        return web.FileResponse(self._resolve_static_file(path))

    async def _bridge(self, request, session):
        socket_path = session["vnc_socket"]
        try:
            reader, writer = await asyncio.open_unix_connection(socket_path)
        except OSError as err:
            raise BrokerError(502, "The desktop session is not reachable.") from err

        client_ws = web.WebSocketResponse(
            protocols=("binary",), heartbeat=30, max_msg_size=0
        )
        await client_ws.prepare(request)

        async def client_to_display():
            async for message in client_ws:
                if message.type == WSMsgType.BINARY:
                    writer.write(message.data)
                    await writer.drain()
                elif message.type == WSMsgType.TEXT:
                    writer.write(message.data.encode("utf-8"))
                    await writer.drain()
                else:
                    break

        async def display_to_client():
            while True:
                chunk = await reader.read(RELAY_CHUNK_BYTES)
                if not chunk:
                    break
                await client_ws.send_bytes(chunk)

        relay_tasks = [
            asyncio.create_task(client_to_display()),
            asyncio.create_task(display_to_client()),
        ]
        try:
            # Whichever direction finishes first ends the session. The other
            # would otherwise block forever on a read that never completes.
            done, pending = await asyncio.wait(
                relay_tasks, return_when=asyncio.FIRST_COMPLETED
            )
            for task in pending:
                task.cancel()
            await asyncio.gather(*pending, return_exceptions=True)
            for task in done:
                relay_error = task.exception()
                if relay_error is not None:
                    LOG.info("Desktop relay ended: %s", relay_error)
        finally:
            writer.close()
            try:
                await writer.wait_closed()
            except OSError:
                pass
            await client_ws.close()
        return client_ws
