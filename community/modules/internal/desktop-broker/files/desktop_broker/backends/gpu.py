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


# Device nodes the NVIDIA kernel driver creates once it is loaded.
NVIDIA_DEVICE_NODES = ("/dev/nvidia0", "/dev/nvidiactl")

# Where libglvnd looks up EGL vendor libraries. The NVIDIA driver registers
# libEGL_nvidia here (10_nvidia.json) only when it is installed with graphics
# support; a compute-only driver leaves no entry.
EGL_VENDOR_DIRS = (
    "/usr/share/glvnd/egl_vendor.d",
    "/etc/glvnd/egl_vendor.d",
)


def nvidia_device_present():
    return any(Path(node).exists() for node in NVIDIA_DEVICE_NODES)


def nvidia_egl_available():
    """Whether the installed NVIDIA driver can render, not merely compute."""
    for directory in EGL_VENDOR_DIRS:
        try:
            if any(Path(directory).glob("*nvidia*.json")):
                return True
        except OSError:
            continue
    return False


def gpu_display():
    """VGL_DISPLAY value naming a usable GPU, or None if there is none.

    VirtualGL's EGL back end accepts an EGL device ID or a DRM card path, but
    rejects a DRM render node with "Invalid EGL device", and the session then
    fails to start. So the render node is never used here, even on images
    whose NVIDIA driver creates one; it is only for backends that take
    "-rendernode" themselves.

    A device with a compute-only driver is not usable: without libEGL_nvidia
    there is no NVIDIA EGL device for VirtualGL to open.
    """
    # EGL access goes through /dev/nvidia*, which is world-readable, so no
    # supplementary group membership is needed (unlike /dev/dri/card*, which is
    # root:video). egl0 is the first enumerated device; these images carry a
    # single GPU.
    if nvidia_device_present() and nvidia_egl_available():
        return "egl0"
    return None


def virtualgl_run():
    return shutil.which("vglrun") or "/opt/VirtualGL/bin/vglrun"


def virtualgl_available():
    return (
        shutil.which("vglrun") is not None
        or Path("/opt/VirtualGL/bin/vglrun").exists()
    )
