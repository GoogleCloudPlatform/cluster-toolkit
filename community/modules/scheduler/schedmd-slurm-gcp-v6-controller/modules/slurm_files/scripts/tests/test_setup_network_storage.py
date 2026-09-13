# Copyright 2026 Google LLC
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

import socket
import sys
from pathlib import Path
from unittest.mock import MagicMock, mock_open, patch

import pytest

PARENT_DIR = str(Path(__file__).resolve().parent.parent)
if PARENT_DIR not in sys.path:
    sys.path.insert(0, PARENT_DIR)

import setup_network_storage
from setup_network_storage import (
    _probe_tcp_port,
    is_controller_mount,
    setup_network_storage as run_setup_network_storage,
    wait_for_controller_nfs,
)
from util import NSMount


def test_probe_tcp_port_success():
    with patch("socket.create_connection") as mock_conn:
        mock_conn.return_value.__enter__.return_value = MagicMock()
        assert _probe_tcp_port("10.0.0.1", port=2049, timeout=1.0) is True
        mock_conn.assert_called_once_with(("10.0.0.1", 2049), timeout=1.0)


def test_probe_tcp_port_failure():
    with patch("socket.create_connection", side_effect=socket.error("Connection refused")):
        assert _probe_tcp_port("10.0.0.1", port=2049, timeout=1.0) is False


def test_probe_tcp_port_empty_or_invalid_host():
    assert _probe_tcp_port("", port=2049) is False
    assert _probe_tcp_port("   ", port=2049) is False
    assert _probe_tcp_port(None, port=2049) is False  # type: ignore


def test_probe_tcp_port_ipv6():
    with patch("socket.create_connection") as mock_conn:
        mock_conn.return_value.__enter__.return_value = MagicMock()
        assert _probe_tcp_port("::1", port=2049, timeout=1.0) is True
        mock_conn.assert_called_once_with(("::1", 2049), timeout=1.0)


def test_probe_tcp_port_os_errors():
    for err in (OSError("Network unreachable"), TypeError("bad arg"), OverflowError("port too large")):
        with patch("socket.create_connection", side_effect=err):
            assert _probe_tcp_port("10.0.0.1", port=2049) is False


def test_wait_for_controller_nfs_invalid_timeout():
    with pytest.raises(TimeoutError, match="Invalid timeout 0s"):
        wait_for_controller_nfs("10.0.0.1", ["/home"], timeout=0)
    with pytest.raises(TimeoutError, match="Invalid timeout -5s"):
        wait_for_controller_nfs("10.0.0.1", ["/home"], timeout=-5)


def test_wait_for_controller_nfs_happy_path():
    with patch("setup_network_storage._probe_tcp_port", return_value=True), \
         patch("time.sleep") as mock_sleep:
        wait_for_controller_nfs("10.0.0.1", ["/home", "/apps"], timeout=10)
        # Settle delay must be called
        mock_sleep.assert_called_once_with(2.0)


def test_wait_for_controller_nfs_retries_port_2049_then_succeeds():
    probe_calls = 0

    def probe_side_effect(host, port=2049, timeout=2.0):
        nonlocal probe_calls
        probe_calls += 1
        return probe_calls >= 3

    with patch("setup_network_storage._probe_tcp_port", side_effect=probe_side_effect), \
         patch("time.sleep") as mock_sleep:
        wait_for_controller_nfs("10.0.0.1", [Path("/home")], timeout=30)
        assert probe_calls == 3
        # Ensure the final settle delay was called
        assert mock_sleep.call_args_list[-1][0] == (2.0,)


def test_wait_for_controller_nfs_timeout():
    # Simulate time advancing past deadline
    fake_time = [100.0]

    def mock_monotonic():
        fake_time[0] += 5.0
        return fake_time[0]

    with patch("setup_network_storage._probe_tcp_port", return_value=False), \
         patch("time.monotonic", side_effect=mock_monotonic), \
         patch("time.sleep"):
        with pytest.raises(TimeoutError, match=r"Timed out after 5s waiting for NFS service on 10\.0\.0\.1:2049"):
            wait_for_controller_nfs("10.0.0.1", ["/home"], timeout=5)


def test_setup_network_storage_controller_skips_preflight():
    mock_lkp = MagicMock()
    mock_lkp.is_controller = True
    home_mount = NSMount(
        server_ip="127.0.0.1",
        remote_mount=Path("/home"),
        local_mount=Path("/home"),
        fs_type="nfs",
        mount_options="_netdev",
    )

    with patch("setup_network_storage.lookup", return_value=mock_lkp), \
         patch("setup_network_storage.resolve_network_storage", return_value=[home_mount]), \
         patch("setup_network_storage.separate", return_value=([], [home_mount])), \
         patch("setup_network_storage.wait_for_controller_nfs") as mock_wait, \
         patch("setup_network_storage.mount_fstab"), \
         patch("setup_network_storage.slurm_key_mount_handler"), \
         patch("setup_network_storage.munge_mount_handler"), \
         patch("pathlib.Path.is_file", return_value=True), \
         patch("shutil.copy2"), \
         patch("builtins.open", mock_open()):
        run_setup_network_storage()
        mock_wait.assert_not_called()


def test_setup_network_storage_client_preflight_includes_key_mounts():
    mock_lkp = MagicMock()
    mock_lkp.is_controller = False
    mock_lkp.cfg.enable_slurm_auth = True
    mock_lkp.slurm_key_mount = NSMount(
        server_ip="10.0.0.1",
        remote_mount=Path("/var/spool/slurm/key"),
        local_mount=Path("/var/spool/slurm/key"),
        fs_type="nfs",
        mount_options="_netdev",
    )
    home_mount = NSMount(
        server_ip="10.0.0.1",
        remote_mount=Path("/home"),
        local_mount=Path("/home"),
        fs_type="nfs",
        mount_options="_netdev",
    )

    with patch("setup_network_storage.lookup", return_value=mock_lkp), \
         patch("setup_network_storage.resolve_network_storage", return_value=[home_mount]), \
         patch("setup_network_storage.is_controller_mount", return_value=True), \
         patch("setup_network_storage.wait_for_controller_nfs") as mock_wait, \
         patch("setup_network_storage.mount_fstab"), \
         patch("setup_network_storage.slurm_key_mount_handler"), \
         patch("setup_network_storage.munge_mount_handler"), \
         patch("pathlib.Path.is_file", return_value=True), \
         patch("shutil.copy2"), \
         patch("util.mkdirp"), \
         patch("builtins.open", mock_open()):
        run_setup_network_storage()
        mock_wait.assert_called_once()
        server_arg, paths_arg = mock_wait.call_args[0][:2]
        assert server_arg == "10.0.0.1"
        assert set(paths_arg) == {"/home", "/var/spool/slurm/key"}


def test_setup_network_storage_client_preflight_failure_aborts_before_fstab():
    mock_lkp = MagicMock()
    mock_lkp.is_controller = False
    mock_lkp.cfg.enable_slurm_auth = False
    mock_lkp.munge_mount = None
    home_mount = NSMount(
        server_ip="10.0.0.1",
        remote_mount=Path("/home"),
        local_mount=Path("/home"),
        fs_type="nfs",
        mount_options="_netdev",
    )

    with patch("setup_network_storage.lookup", return_value=mock_lkp), \
         patch("setup_network_storage.resolve_network_storage", return_value=[home_mount]), \
         patch("setup_network_storage.is_controller_mount", return_value=True), \
         patch("setup_network_storage.wait_for_controller_nfs", side_effect=TimeoutError("Controller down")), \
         patch("shutil.copy2") as mock_copy, \
         patch("builtins.open", mock_open()) as mock_fstab:
        with pytest.raises(TimeoutError, match="Controller down"):
            run_setup_network_storage()
        # Verify /etc/fstab was never copied or opened
        mock_copy.assert_not_called()
        mock_fstab.assert_not_called()


def test_is_controller_mount_skips_dns_for_control_host():
    mock_lkp = MagicMock()
    mock_lkp.is_controller = False
    mock_lkp.control_host = "slurm-controller"
    mock_lkp.control_host_addr = "10.0.0.2"

    mount = NSMount(
        server_ip="slurm-controller",
        remote_mount=Path("/home"),
        local_mount=Path("/home"),
        fs_type="nfs",
        mount_options="_netdev",
    )

    with patch("setup_network_storage.lookup", return_value=mock_lkp), \
         patch("util.host_lookup") as mock_host_lookup:
        assert is_controller_mount(mount) is True
        # Ensure expensive DNS resolution with retry backoff is NEVER called
        mock_host_lookup.assert_not_called()


def test_is_controller_mount_skips_dns_for_control_host_addr():
    mock_lkp = MagicMock()
    mock_lkp.is_controller = False
    mock_lkp.control_host = "slurm-controller"
    mock_lkp.control_host_addr = "10.0.0.2"

    mount = NSMount(
        server_ip="10.0.0.2",
        remote_mount=Path("/home"),
        local_mount=Path("/home"),
        fs_type="nfs",
        mount_options="_netdev",
    )

    with patch("setup_network_storage.lookup", return_value=mock_lkp), \
         patch("util.host_lookup") as mock_host_lookup:
        assert is_controller_mount(mount) is True
        mock_host_lookup.assert_not_called()


def test_setup_network_storage_preflight_short_circuits_is_controller_mount():
    mock_lkp = MagicMock()
    mock_lkp.is_controller = False
    mock_lkp.control_host = "slurm-controller"
    mock_lkp.control_host_addr = "10.0.0.2"
    mock_lkp.cfg.enable_slurm_auth = False
    mock_lkp.munge_mount = None

    home_mount = NSMount(
        server_ip="slurm-controller",
        remote_mount=Path("/home"),
        local_mount=Path("/home"),
        fs_type="nfs",
        mount_options="_netdev",
    )

    with patch("setup_network_storage.lookup", return_value=mock_lkp), \
         patch("setup_network_storage.resolve_network_storage", return_value=[home_mount]), \
         patch("setup_network_storage.is_controller_mount") as mock_is_controller_mount, \
         patch("setup_network_storage.wait_for_controller_nfs") as mock_wait, \
         patch("setup_network_storage.mount_fstab"), \
         patch("setup_network_storage.munge_mount_handler"), \
         patch("pathlib.Path.is_file", return_value=True), \
         patch("shutil.copy2"), \
         patch("util.mkdirp"), \
         patch("builtins.open", mock_open()):
        run_setup_network_storage()
        # When server matches control_host, is_controller_mount must be short-circuited
        mock_is_controller_mount.assert_not_called()
        mock_wait.assert_called_once_with("slurm-controller", {"/home"}, timeout=360)
