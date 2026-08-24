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


class TigerVNCBackend(VNCBackend):
    name = "tigervnc"

    tcp_disabled_port = -1
    security_args = ("-UseBlacklist=0", "-SecurityTypes", "None")

    def vncserver_command(self):
        return gpu.which("vncserver", "/usr/bin/vncserver")

    def render_node_args(self, render_node):
        return ["-rendernode", str(render_node)]

    # This backend's only GL offload is "-rendernode", so unlike the base class
    # an EGL device is no use to it: acceleration requires a real DRM render
    # node. Without this override it would report accelerated() on a host where
    # it cannot in fact accelerate anything.
    def accelerated(self):
        return self.gpu_acceleration and gpu.first_render_node() is not None

    def acceleration_diagnostic(self):
        if not self.gpu_acceleration:
            return None
        if gpu.first_render_node() is None:
            return (
                "this backend offloads GL only through '-rendernode', which "
                "needs a DRM render node, and none is present - GCE's NVIDIA "
                "images build nvidia_drm without KMS so /dev/dri is never "
                'created. Set vnc_backend="turbovnc" for GPU desktops.'
            )
        return None
