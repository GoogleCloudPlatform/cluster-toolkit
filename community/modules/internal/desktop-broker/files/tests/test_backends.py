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

SESSION = {
    "display_number": 1,
    "vnc_socket": "/run/x/1/vnc.sock",
    "xstartup": "/run/x/1/xstartup",
}


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

    # A GPU present as an EGL device but no DRM render node. TurboVNC can use
    # it through VirtualGL, TigerVNC cannot.
    monkeypatch.setattr(gpu, "first_render_node", lambda: None)
    monkeypatch.setattr(gpu, "gpu_display", lambda: "egl0")
    monkeypatch.setattr(gpu, "virtualgl_available", lambda: True)

    tiger = backends.create("tigervnc", "1920x1080", gpu_acceleration=True)
    assert tiger.accelerated() is False
    assert "rendernode" in tiger.acceleration_diagnostic()

    turbo = backends.create("turbovnc", "1920x1080", gpu_acceleration=True)
    assert turbo.accelerated() is True


@pytest.fixture
def gpu_host(monkeypatch, tmp_path):
    """Point GPU detection at a scratch directory instead of the real host.

    Returns a function that describes the host: whether the NVIDIA device
    nodes exist and whether the driver registered its EGL library.
    """
    from desktop_broker.backends import gpu

    device = tmp_path / "dev" / "nvidia0"
    vendor_dir = tmp_path / "egl_vendor.d"
    device.parent.mkdir()
    vendor_dir.mkdir()
    monkeypatch.setattr(gpu, "NVIDIA_DEVICE_NODES", (str(device),))
    monkeypatch.setattr(gpu, "EGL_VENDOR_DIRS", (str(vendor_dir),))
    # A render node is present throughout. gpu_display() must ignore it: these
    # tests fail if it is ever preferred again.
    monkeypatch.setattr(
        gpu, "first_render_node", lambda: gpu.Path("/dev/dri/renderD128")
    )

    def describe(device_present, graphics_driver):
        if device_present:
            device.touch()
        # Mesa registers itself on every host; only the NVIDIA entry counts.
        (vendor_dir / "50_mesa.json").touch()
        if graphics_driver:
            (vendor_dir / "10_nvidia.json").touch()
        return gpu

    return describe


def test_virtualgl_gets_the_egl_device_despite_a_render_node(gpu_host):
    """VirtualGL rejects a DRM render node, so one must never be handed to it.

    Images whose NVIDIA driver creates /dev/dri/renderD128 used to get that
    path as VGL_DISPLAY, and the session died with "Invalid EGL device".
    """
    gpu = gpu_host(device_present=True, graphics_driver=True)
    assert gpu.gpu_display() == "egl0"


def test_no_nvidia_device_means_no_virtualgl_device(gpu_host):
    gpu = gpu_host(device_present=False, graphics_driver=True)
    assert gpu.gpu_display() is None

    backend = backends.create("turbovnc", "1920x1080", gpu_acceleration=True)
    assert backend.accelerated() is False
    assert "no NVIDIA device" in backend.acceleration_diagnostic()


def test_compute_only_driver_falls_back_and_says_why(gpu_host, monkeypatch):
    # /dev/nvidia* exists with a compute-only driver too, but there is no
    # libEGL_nvidia for VirtualGL to render with.
    gpu = gpu_host(device_present=True, graphics_driver=False)
    monkeypatch.setattr(gpu, "virtualgl_available", lambda: True)
    assert gpu.gpu_display() is None

    backend = backends.create("turbovnc", "1920x1080", gpu_acceleration=True)
    assert backend.accelerated() is False
    assert "compute-only" in backend.acceleration_diagnostic()
    assert backend.session_command() == ["dbus-launch", "startxfce4"]
    assert backend.session_environment() == {}
    assert backend.environment() == {"LIBGL_ALWAYS_SOFTWARE": "1"}


def test_turbovnc_without_virtualgl_falls_back_and_says_why(
    gpu_host, monkeypatch
):
    # The VirtualGL install is non-fatal, so a GPU host can come up without it.
    gpu = gpu_host(device_present=True, graphics_driver=True)
    monkeypatch.setattr(gpu, "virtualgl_available", lambda: False)

    backend = backends.create("turbovnc", "1920x1080", gpu_acceleration=True)
    assert backend.accelerated() is False
    assert "VirtualGL is not installed" in backend.acceleration_diagnostic()
    assert backend.session_command() == ["dbus-launch", "startxfce4"]
    assert backend.session_environment() == {}
    assert backend.environment() == {"LIBGL_ALWAYS_SOFTWARE": "1"}


def test_tigervnc_accelerates_through_a_render_node(gpu_host):
    gpu_host(device_present=True, graphics_driver=True)

    backend = backends.create("tigervnc", "1920x1080", gpu_acceleration=True)
    assert backend.accelerated() is True
    assert backend.acceleration_diagnostic() is None
    assert backend.environment() == {}


def test_tigervnc_does_not_trust_a_compute_only_render_node(gpu_host):
    # The NVIDIA kernel driver creates the render node even when the image
    # has a compute-only driver, so the node alone proves nothing.
    gpu_host(device_present=True, graphics_driver=False)

    backend = backends.create("tigervnc", "1920x1080", gpu_acceleration=True)
    assert backend.accelerated() is False
    assert "compute-only" in backend.acceleration_diagnostic()
    assert backend.environment() == {"LIBGL_ALWAYS_SOFTWARE": "1"}


def test_tigervnc_accelerates_on_a_render_node_without_nvidia(gpu_host):
    # The compute-only check is about NVIDIA drivers; a render node from any
    # other GPU is still usable.
    gpu_host(device_present=False, graphics_driver=False)

    backend = backends.create("tigervnc", "1920x1080", gpu_acceleration=True)
    assert backend.accelerated() is True
    assert backend.acceleration_diagnostic() is None


def test_tigervnc_stays_quiet_when_gpu_is_off(gpu_host):
    gpu_host(device_present=True, graphics_driver=False)

    backend = backends.create("tigervnc", "1920x1080", gpu_acceleration=False)
    assert backend.accelerated() is False
    assert backend.acceleration_diagnostic() is None


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


@pytest.mark.parametrize("name", ["tigervnc", "turbovnc"])
def test_session_uses_the_node_local_xstartup(name):
    # Not ~/.vnc/xstartup, which is shared between nodes with different
    # backends when home is on NFS.
    command = make(name).start_command(SESSION)
    assert command[command.index("-xstartup") + 1] == "/run/x/1/xstartup"
