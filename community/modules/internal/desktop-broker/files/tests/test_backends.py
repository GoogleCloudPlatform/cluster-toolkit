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

"""VNC backend command construction.

The transport flags are security-relevant: a missing "-rfbport -1" leaves
TigerVNC's TCP listener bound, which would make the display reachable by any
local user on a shared login node instead of only through its 0700 socket.
"""

import pytest

from desktop_broker.backends import registry as backends

SESSION = {"display_number": 1, "vnc_socket": "/run/x/1/vnc.sock"}


def make(name):
    return backends.create(name, "1920x1080")


def test_unknown_backend_rejected():
    with pytest.raises(ValueError, match="Unsupported VNC backend"):
        backends.create("realvnc", "1920x1080")


@pytest.mark.parametrize("name", ["tigervnc", "turbovnc"])
def test_display_listens_on_a_socket_with_no_password(name):
    command = " ".join(make(name).start_command(SESSION))
    assert "-rfbunixpath /run/x/1/vnc.sock" in command
    # Access control is the 0700 socket directory, so there is nothing to
    # authenticate against and no password to leak.
    assert "-rfbauth" not in command


def test_tigervnc_closes_its_tcp_listener():
    # "-rfbunixpath" ADDS a socket and leaves TCP bound; only "-rfbport -1"
    # closes it. Losing this silently exposes the display to other local users.
    command = make("tigervnc").start_command(SESSION)
    assert "-rfbport" in command
    assert command[command.index("-rfbport") + 1] == "-1"
    assert "None" in command  # -SecurityTypes None


def test_turbovnc_needs_no_port_flag():
    # TurboVNC suppresses TCP on its own and refuses to start with -rfbport -1.
    assert "-rfbport" not in make("turbovnc").start_command(SESSION)


def test_turbovnc_disables_rfb_authentication():
    command = " ".join(make("turbovnc").start_command(SESSION))
    assert "-securitytypes none" in command


def test_gpu_off_means_no_acceleration_and_no_complaint():
    backend = backends.create("turbovnc", "1920x1080", gpu_acceleration=False)
    assert backend.accelerated() is False
    assert backend.acceleration_diagnostic() is None


def test_tigervnc_explains_it_cannot_use_an_egl_device(monkeypatch):
    from desktop_broker.backends import gpu

    # A GPU present as an EGL device but no DRM render node: the GCE NVIDIA
    # image case. TurboVNC can use it through VirtualGL, TigerVNC cannot.
    monkeypatch.setattr(gpu, "first_render_node", lambda: None)
    monkeypatch.setattr(gpu, "gpu_display", lambda: "egl0")

    tiger = backends.create("tigervnc", "1920x1080", gpu_acceleration=True)
    assert tiger.accelerated() is False
    assert "rendernode" in tiger.acceleration_diagnostic()

    turbo = backends.create("turbovnc", "1920x1080", gpu_acceleration=True)
    assert turbo.accelerated() is True


def test_software_rendering_is_forced_when_not_accelerated():
    backend = backends.create("turbovnc", "1920x1080", gpu_acceleration=False)
    assert backend.environment() == {"LIBGL_ALWAYS_SOFTWARE": "1"}


def test_glx_disabled_when_no_render_node(monkeypatch):
    from desktop_broker.backends import gpu

    # Without this flag Xvnc segfaults on a GPU-less host that carries NVIDIA
    # driver libraries.
    monkeypatch.setattr(gpu, "first_render_node", lambda: None)
    command = make("tigervnc").start_command(SESSION)
    assert command[-2:] == ["-extension", "GLX"]


def test_session_command_is_the_xfce_desktop():
    assert make("tigervnc").session_command() == ["dbus-launch", "startxfce4"]
