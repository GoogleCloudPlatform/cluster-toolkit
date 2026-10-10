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

"""Detecting what hardware OpenGL is actually available."""

import shutil
from pathlib import Path


def which(binary_name, fallback):
    return shutil.which(binary_name) or fallback


def first_render_node():
    try:
        # /dev/dri holds only cardN and renderDN, so "render*" selects the
        # render nodes exactly.
        return next(iter(sorted(Path("/dev/dri").glob("render*"))), None)
    except OSError:
        return None


def gpu_display():
    """VGL_DISPLAY value naming a usable GPU, or None if there is none.

    Prefers a DRM render node when one exists, because it names the device
    unambiguously. GCE's NVIDIA images do not provide one - nvidia_drm is built
    without KMS, exposes no "modeset" parameter and creates no /dev/dri - so
    fall back to an EGL device ID, which VirtualGL also accepts and which is
    enumerated through EGL itself with no dependency on DRM.
    """
    render_node = first_render_node()
    if render_node:
        return str(render_node)
    # EGL access goes through /dev/nvidia*, which is world-readable, so no
    # supplementary group membership is needed (unlike /dev/dri, which is
    # root:render). egl0 is the first enumerated device; these images carry a
    # single GPU.
    if Path("/dev/nvidia0").exists() or Path("/dev/nvidiactl").exists():
        return "egl0"
    return None


def virtualgl_run():
    return shutil.which("vglrun") or "/opt/VirtualGL/bin/vglrun"


def virtualgl_available():
    return (
        shutil.which("vglrun") is not None
        or Path("/opt/VirtualGL/bin/vglrun").exists()
    )
