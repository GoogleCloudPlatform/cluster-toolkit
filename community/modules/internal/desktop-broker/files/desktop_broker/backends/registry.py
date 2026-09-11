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

"""Choosing a VNC server flavour.

Named rather than living in a package __init__: gcluster embeds these modules
into its binary with go:embed, which silently skips any file whose name starts
with an underscore. An __init__.py would never reach a deployed host, so this
package relies on PEP 420 implicit namespace packages instead.
"""

from .base import VNCBackend
from .tigervnc import TigerVNCBackend
from .turbovnc import TurboVNCBackend

BACKENDS = {
    TigerVNCBackend.name: TigerVNCBackend,
    TurboVNCBackend.name: TurboVNCBackend,
}

__all__ = ["VNCBackend", "TigerVNCBackend", "TurboVNCBackend", "create"]


def create(name, session_resolution, gpu_acceleration=False):
    normalized = str(name or "").strip().lower()
    try:
        backend_class = BACKENDS[normalized]
    except KeyError as err:
        raise ValueError(
            f"Unsupported VNC backend: {name!r}. Expected one of: "
            f"{', '.join(sorted(BACKENDS))}."
        ) from err
    return backend_class(
        session_resolution=session_resolution,
        gpu_acceleration=gpu_acceleration,
    )
