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

from . import gpu
from .base import VNCBackend


class TurboVNCBackend(VNCBackend):
    name = "turbovnc"

    security_args = ("-securitytypes", "none")

    def vncserver_command(self):
        return gpu.which("vncserver", "/opt/TurboVNC/bin/vncserver")

    def acceleration_diagnostic(self):
        reason = super().acceleration_diagnostic()
        if reason or not self.gpu_acceleration:
            return reason
        if not gpu.virtualgl_available():
            return (
                "a GPU is present but VirtualGL is not installed, so GL cannot "
                "be redirected to it. Check the desktop runtime setup step: the "
                "VirtualGL install is deliberately non-fatal, so it warns and "
                "continues rather than failing the whole runtime."
            )
        return None

    # TurboVNC's Xvnc takes no "-rendernode", so render_node_args stays empty
    # and hardware GL is obtained a different way: VirtualGL interposes GLX
    # calls from applications and executes them on the GPU.
    def session_command(self):
        base = super().session_command()
        if not self.accelerated() or not gpu.virtualgl_available():
            # Asked for acceleration but VirtualGL is not installed, or no
            # usable device. Fall back to software rather than producing a
            # session that will not start.
            return base
        # "+wm" is required when wrapping a whole desktop session rather than a
        # single application; without it VirtualGL does not interpose the
        # children the window manager launches.
        return [gpu.virtualgl_run(), "+wm"] + base

    def session_environment(self):
        if not self.accelerated() or not gpu.virtualgl_available():
            return {}
        gpu_display = gpu.gpu_display()
        if not gpu_display:
            return {}
        # VirtualGL's EGL back end. Deliberately *not* the classic GLX back end:
        # that would need a second X server bound to the GPU plus nvidia-xconfig
        # and vglserver_config, all of it long-lived host infrastructure. The EGL
        # back end renders straight to the device, so the broker keeps starting
        # one Xvnc per user ad hoc and nothing else.
        return {"VGL_DISPLAY": gpu_display}
