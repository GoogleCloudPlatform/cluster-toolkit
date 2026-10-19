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

from contextlib import contextmanager
import socket
import sys
from pathlib import Path
from typing import Optional
from unittest.mock import MagicMock, mock_open, patch

PARENT_DIR = str(Path(__file__).resolve().parent.parent)
if PARENT_DIR not in sys.path:
    sys.path.insert(0, PARENT_DIR)

import setup_network_storage
from setup_network_storage import (
    _check_nfs_exports_showmount,
    _is_controller_nfs_server,
    _is_path_exported,
    _probe_tcp_port,
    setup_network_storage as run_setup_network_storage,
    wait_for_controller_nfs,
)
from util import NSDict, NSMount


def _mnt(server="10.0.0.1", path="/home", fs_type="nfs"):
    return NSMount(server_ip=server, remote_mount=Path(path), local_mount=Path(path), fs_type=fs_type, mount_options="_netdev")


@contextmanager
def _client_env(mounts, lkp=None):
    if lkp is None:
        lkp = MagicMock(is_controller=False, control_host="slurm-controller", control_addr="10.0.0.1")
        lkp.controller_mount_server_ip.return_value = "10.0.0.1"
        lkp.cfg = NSDict(enable_slurm_auth=False)
        lkp.munge_mount = None
    with patch("setup_network_storage.lookup", return_value=lkp), \
         patch("setup_network_storage.resolve_network_storage", return_value=mounts), \
         patch("setup_network_storage.wait_for_controller_nfs", return_value=True) as mock_wait, \
         patch("setup_network_storage.mount_fstab") as mock_mount_fstab, \
         patch("setup_network_storage.slurm_key_mount_handler"), \
         patch("setup_network_storage.munge_mount_handler"), \
         patch("pathlib.Path.is_file", return_value=True), \
         patch("shutil.copy2") as mock_copy, \
         patch("util.mkdirp"), \
         patch("builtins.open", mock_open()) as mock_fstab:
        yield lkp, mock_wait, mock_mount_fstab, mock_copy, mock_fstab


def test_probe_tcp_port():
    with patch("socket.create_connection") as mock_conn:
        mock_conn.return_value.__enter__.return_value = MagicMock()
        assert _probe_tcp_port("10.0.0.1", port=2049, timeout=1.0) is True
        assert _probe_tcp_port("::1", port=2049, timeout=1.0) is True
    for invalid in ("", "   ", None):
        assert _probe_tcp_port(invalid, port=2049) is False  # type: ignore
    for err in (OSError("refused"), TypeError("bad"), OverflowError("overflow")):
        with patch("socket.create_connection", side_effect=err):
            assert _probe_tcp_port("10.0.0.1", port=2049) is False


def test_is_path_exported():
    active = {"/home", "/opt/apps"}
    for valid in ("/home", "/home/", "/home/shared", "//home/shared", "/opt/apps/cuda/12.0"):
        assert _is_path_exported(valid, active) is True
    for rejected in ("/home2", "/home_other", "/opt/apps2", "/scratch", "", "relative/path"):
        assert _is_path_exported(rejected, active) is False
    assert _is_path_exported("", {""}) is False
    assert _is_path_exported("/home", {""}) is False
    assert _is_path_exported("/any/nested/path", {"/"}) is True
    assert _is_path_exported("/home", {"//"}) is True


def test_check_nfs_exports_showmount():
    out = "Export list for 10.0.0.1:\n/home *\n/opt/apps 10.0.0.0/16\n"
    with patch("shutil.which", return_value="/usr/sbin/showmount"), \
         patch("setup_network_storage._probe_tcp_port", return_value=True), \
         patch("setup_network_storage.run", return_value=MagicMock(returncode=0, stdout=out)):
        assert _check_nfs_exports_showmount("10.0.0.1", {"/home", "/opt/apps"}) is True
        assert _check_nfs_exports_showmount("10.0.0.1", {"/home/shared", "/opt/apps/cuda/12.0"}) is True
        assert _check_nfs_exports_showmount("10.0.0.1", {"/home", "/scratch"}) is False
        assert _check_nfs_exports_showmount("10.0.0.1", {"/home_other"}) is False

    with patch("shutil.which", return_value=None):
        assert _check_nfs_exports_showmount("10.0.0.1", {"/home"}) is None

    with patch("shutil.which", return_value="/usr/sbin/showmount"), \
         patch("setup_network_storage._probe_tcp_port", return_value=False), \
         patch("setup_network_storage.run") as mock_run:
        assert _check_nfs_exports_showmount("10.0.0.1", {"/home"}) is None
        mock_run.assert_not_called()

    for side_effect in ([MagicMock(returncode=1, stderr="err")], RuntimeError("boom")):
        with patch("shutil.which", return_value="/usr/sbin/showmount"), \
             patch("setup_network_storage._probe_tcp_port", return_value=True), \
             patch("setup_network_storage.run", side_effect=side_effect):
            assert _check_nfs_exports_showmount("10.0.0.1", {"/home"}) is None


def test_wait_for_controller_nfs():
    with patch("setup_network_storage.log.warning") as mock_warn:
        assert wait_for_controller_nfs("10.0.0.1", ["/home"], timeout=0) is False
        assert wait_for_controller_nfs("10.0.0.1", ["/home"], timeout=-5) is False
        assert mock_warn.call_count == 2

    cases: list[tuple[Optional[bool], list[str]]] = [(True, ["/home", "/apps"]), (None, ["/home"]), (True, [])]
    for showmount_ret, paths in cases:
        with patch("setup_network_storage._probe_tcp_port", return_value=True), \
             patch("setup_network_storage._check_nfs_exports_showmount", return_value=showmount_ret), \
             patch("time.sleep") as mock_sleep:
            assert wait_for_controller_nfs("10.0.0.1", paths, timeout=10) is True
            mock_sleep.assert_called_once_with(0.5)

    with patch("setup_network_storage._probe_tcp_port", return_value=True), \
         patch("setup_network_storage._check_nfs_exports_showmount", side_effect=[False, False, True]) as mock_chk, \
         patch("time.sleep"):
        assert wait_for_controller_nfs("10.0.0.1", ["/home"], timeout=30) is True
        assert mock_chk.call_count == 3


def test_wait_for_controller_nfs_timeouts():
    t = [100.0]
    with patch("setup_network_storage._probe_tcp_port", return_value=False), \
         patch("time.monotonic", side_effect=lambda: t.__setitem__(0, t[0] + 5.0) or t[0]), \
         patch("time.sleep"), \
         patch("setup_network_storage.log.warning") as mock_warn:
        assert wait_for_controller_nfs("10.0.0.1", ["/home"], timeout=5) is False
        assert "NFS port 2049" in mock_warn.call_args[0][0]

    with patch("setup_network_storage._probe_tcp_port", return_value=True), \
         patch("setup_network_storage._check_nfs_exports_showmount", return_value=False), \
         patch("time.monotonic", side_effect=lambda: t.__setitem__(0, t[0] + 5.0) or t[0]), \
         patch("time.sleep"), \
         patch("setup_network_storage.log.warning") as mock_warn:
        assert wait_for_controller_nfs("10.0.0.1", ["/home"], timeout=5) is False
        assert "to export ['/home']" in mock_warn.call_args[0][0]


def test_is_controller_nfs_server():
    lkp = setup_network_storage.util.Lookup(NSDict(slurm_control_host="slurm-controller", slurm_control_addr="10.0.0.2"))
    with patch("util.host_lookup") as mock_hl, patch("socket.gethostbyname") as mock_ghbn:
        assert _is_controller_nfs_server("", lkp) is False
        assert _is_controller_nfs_server("slurm-controller", lkp) is True
        assert _is_controller_nfs_server("10.0.0.2", lkp) is True
        mock_hl.assert_not_called()
        mock_ghbn.assert_not_called()

    with patch("util.host_lookup") as mock_hl, \
         patch("socket.gethostbyname", side_effect=socket.gaierror("no host")) as mock_ghbn:
        assert _is_controller_nfs_server("ext.internal", lkp) is False
        mock_ghbn.assert_called_once_with("ext.internal")
        mock_hl.assert_not_called()

    with patch("util.host_lookup") as mock_hl, \
         patch("socket.gethostbyname", side_effect=lambda h: "10.99.0.5" if h == "filestore.internal" else "10.0.0.2"):
        assert _is_controller_nfs_server("filestore.internal", lkp) is False
        assert _is_controller_nfs_server("controller-alias.internal", lkp) is True
        mock_hl.assert_not_called()

    lkp_no_addr = setup_network_storage.util.Lookup(NSDict(slurm_control_host="slurm-controller", slurm_control_addr=None))
    with patch("util.host_lookup") as mock_hl, patch("socket.gethostbyname", return_value="10.0.0.2"):
        assert _is_controller_nfs_server("10.0.0.2", lkp_no_addr) is True
        mock_hl.assert_not_called()


def test_setup_network_storage_controller_and_client_preflight():
    with _client_env([_mnt(server="127.0.0.1")], lkp=MagicMock(is_controller=True)) as (_, mock_wait, _, _, _), \
         patch("setup_network_storage.separate", return_value=([], [_mnt(server="127.0.0.1")])):
        run_setup_network_storage()
        mock_wait.assert_not_called()

    with _client_env([_mnt(path="/home", fs_type="NFS"), _mnt(path="/apps", fs_type="Nfs")]) as (lkp, mock_wait, _, _, _):
        lkp.cfg.enable_slurm_auth = True
        lkp.slurm_key_mount = _mnt(path="/var/spool/slurm/key")
        run_setup_network_storage()
        mock_wait.assert_called_once_with("10.0.0.1", {"/home", "/apps", "/var/spool/slurm/key"}, timeout=600)


def test_setup_network_storage_timeout_scaling_and_fallback():
    with _client_env([_mnt()]) as (lkp, mock_wait, _, _, _):
        run_setup_network_storage()
        mock_wait.assert_called_once_with("10.0.0.1", {"/home"}, timeout=600)
        assert "controller_startup_scripts_timeout" not in lkp.cfg

    for cfg_val, expected in ((None, 600), (True, 600), (120, 600), (300, 600), (900, 1200)):
        with _client_env([_mnt()]) as (lkp, mock_wait, _, _, _):
            lkp.cfg = NSDict(enable_slurm_auth=False, controller_startup_scripts_timeout=cfg_val)
            run_setup_network_storage()
            mock_wait.assert_called_once_with("10.0.0.1", {"/home"}, timeout=expected)

    with _client_env([_mnt()]) as (_, mock_wait, mock_mount_fstab, mock_copy, mock_fstab):
        mock_wait.return_value = False
        run_setup_network_storage()
        assert mock_copy.called and mock_fstab.called
        mock_mount_fstab.assert_called_once()


def test_setup_network_storage_external_nfs_server_avoids_host_lookup_retries():
    lkp = setup_network_storage.util.Lookup(NSDict(slurm_control_host="slurm-controller", enable_slurm_auth=False))
    mounts = [_mnt(server="unresolvable-filestore.local", path="/datasets"), _mnt(server="slurm-controller", path="/home")]
    with patch.object(setup_network_storage.util.Lookup, "is_controller", new=False), \
         _client_env(mounts, lkp=lkp) as (_, mock_wait, _, _, _), \
         patch("setup_network_storage.is_controller_mount") as mock_icm, \
         patch("util.host_lookup") as mock_hl, \
         patch("socket.gethostbyname", side_effect=socket.gaierror("DNS failed")) as mock_ghbn:
        run_setup_network_storage()
        mock_icm.assert_not_called()
        mock_hl.assert_not_called()
        mock_ghbn.assert_called_once_with("unresolvable-filestore.local")
        mock_wait.assert_called_once_with("slurm-controller", {"/home", "/etc/munge"}, timeout=600)
