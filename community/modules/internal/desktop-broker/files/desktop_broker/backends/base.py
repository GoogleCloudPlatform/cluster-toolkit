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

"""What every VNC server flavour has to provide.

Each display listens on a 0700 unix socket with no TCP listener and no RFB
password. Access control is filesystem permission, enforced by the kernel on
connect(), and the broker - which relays RFB itself - is the only
network-reachable way in.
"""

from . import gpu


class VNCBackend:
    name = None

    # "-rfbport" value that closes the TCP listener, or None when the backend
    # needs no such flag.
    #
    # TigerVNC: "-rfbunixpath" ADDS a socket and leaves TCP bound, so only
    # "-rfbport -1" closes it. Dropping that flag silently leaves the display
    # reachable by any local user - it is load-bearing.
    #
    # TurboVNC: "-rfbunixpath" suppresses TCP on its own. Verified on a live
    # node: even an explicit positive "-rfbport" does not bind. It also refuses
    # to start with "-rfbport -1", so it gets no flag at all.
    tcp_disabled_port = None

    # How this flavour spells "no RFB authentication".
    security_args = ()

    def __init__(self, session_resolution, gpu_acceleration=False):
        self.session_resolution = session_resolution
        self.gpu_acceleration = bool(gpu_acceleration)

    # -- binaries ----------------------------------------------------------

    def vncserver_command(self):
        raise NotImplementedError

    def kill_command(self, display_number):
        return [self.vncserver_command(), "-kill", f":{display_number}"]

    def render_node_args(self, render_node):
        return []

    def accelerated(self):
        """Whether hardware GL should be used.

        Requires both that the module asked for it and that a device this
        backend can actually use is present. On a host with no GPU, or with the
        driver absent, the software path must be used instead.
        """
        return self.gpu_acceleration and gpu.gpu_display() is not None

    def acceleration_diagnostic(self):
        """Why requested hardware GL will not happen, or None if it will.

        Reported so the broker can say so at startup: an unexplained fall back
        to software rendering is very hard to tell apart from a slow desktop.
        """
        if not self.gpu_acceleration:
            return None
        if gpu.gpu_display() is None:
            return (
                "no usable GPU device was found (neither a /dev/dri render node "
                "nor /dev/nvidia*). The image most likely has no NVIDIA driver, "
                "or only a compute driver without graphics support - a compute "
                "driver ships no libEGL_nvidia and cannot render."
            )
        return None

    # -- the desktop session ----------------------------------------------

    def session_command(self):
        """Argv for the desktop session, as written into ~/.vnc/xstartup."""
        return ["dbus-launch", "startxfce4"]

    def session_environment(self):
        """Extra environment for the desktop session (not the X server)."""
        return {}

    def environment(self):
        """Extra environment for the vncserver invocation.

        Keyed on accelerated(), not on the presence of a render node. Xvnc's
        children inherit this, and xstartup is one of them, so forcing software
        Mesa here would contradict the session's own hardware GL setup: on GCE
        there is no render node even when a GPU is present and VirtualGL is
        rendering on it.

        NOTE: this does NOT prevent the Xvnc crash described in start_command.
        LIBGL_ALWAYS_SOFTWARE only affects client-side Mesa - the applications -
        not the X server's own driver probing; "-extension GLX" is what keeps
        the server alive. It is set purely so session apps on a GPU-less host do
        not attempt hardware GL.
        """
        if self.accelerated():
            return {}
        return {"LIBGL_ALWAYS_SOFTWARE": "1"}

    # -- starting ----------------------------------------------------------

    def transport_args(self, session):
        return [
            "-rfbunixpath",
            str(session["vnc_socket"]),
            *(
                # See tcp_disabled_port. The vncserver wrapper injects its own
                # "-rfbport <5900+display>", so this must come later on the
                # command line to win.
                ["-rfbport", str(self.tcp_disabled_port)]
                if self.tcp_disabled_port is not None
                else []
            ),
            *self.security_args,
        ]

    def start_command(self, session):
        command = [
            self.vncserver_command(),
            f":{session['display_number']}",
            "-geometry",
            self.session_resolution,
            *self.transport_args(session),
        ]
        render_node = gpu.first_render_node()
        if render_node:
            command.extend(self.render_node_args(render_node))
        else:
            # On a host that has the NVIDIA driver libraries but no device, Xvnc
            # segfaults while initialising GLX. Verified on a GPU-less n2
            # carrying libEGL_nvidia 550.90.12 with no /dev/dri: both backends
            # die with "Caught signal 11" without this flag and start cleanly
            # with it. DRI3 cannot be disabled the same way - these builds
            # reject "-extension DRI3" and initialise it regardless.
            command.extend(["-extension", "GLX"])
        return command
