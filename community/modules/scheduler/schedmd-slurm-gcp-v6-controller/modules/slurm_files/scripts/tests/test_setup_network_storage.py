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
import pytest
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


def _controller_lkp(**cfg):
    lkp = MagicMock(is_controller=True)
    lkp.cfg = NSDict(enable_slurm_auth=True, **cfg)
    lkp.slurm_key_mount = _mnt(server=None, path="/slurm/key_distribution")
    return lkp


def test_nfs_export_clients_uses_config_and_drops_wildcard():
    lkp = _controller_lkp(nfs_export_clients=["10.0.0.0/24", "*", "", "10.0.0.5/24", "bogus", "10.1.0.0/16"])
    with patch("util.lookup", return_value=lkp):
        assert setup_network_storage.nfs_export_clients() == ["10.0.0.0/24", "10.1.0.0/16"]


@pytest.mark.parametrize("broad", ["0.0.0.0/0", "::/0", "0.0.0.0/1", "128.0.0.0/1", "10.0.0.0/7", "2001:db8::/31"])
def test_nfs_export_clients_rejects_broad_ranges(broad):
    lkp = _controller_lkp(nfs_export_clients=[broad, "10.0.0.0/8", "2001:db8::/32"])
    with patch("util.lookup", return_value=lkp):
        assert setup_network_storage.nfs_export_clients() == ["10.0.0.0/8", "2001:db8::/32"]


def test_nfs_export_clients_only_broad_ranges_falls_back_to_controller_subnet():
    lkp = _controller_lkp(nfs_export_clients=["0.0.0.0/1", "128.0.0.0/1"])
    md = {"network-interfaces/0/ip": "10.2.3.4", "network-interfaces/0/subnetmask": "255.255.255.0"}
    with patch("util.lookup", return_value=lkp), \
         patch("util.instance_metadata", side_effect=lambda p, silent=False: md[p]):
        assert setup_network_storage.nfs_export_clients() == ["10.2.3.0/24"]


def test_nfs_export_clients_falls_back_to_controller_subnet():
    lkp = _controller_lkp()
    md = {"network-interfaces/0/ip": "10.2.3.4", "network-interfaces/0/subnetmask": "255.255.240.0"}
    with patch("util.lookup", return_value=lkp), \
         patch("util.instance_metadata", side_effect=lambda p, silent=False: md[p]):
        assert setup_network_storage.nfs_export_clients() == ["10.2.0.0/20"]


def test_nfs_export_clients_never_wildcard_when_unknown():
    lkp = _controller_lkp(nfs_export_clients=["*"])
    with patch("util.lookup", return_value=lkp), \
         patch("util.instance_metadata", side_effect=Exception("no metadata")):
        with pytest.raises(RuntimeError):
            setup_network_storage.nfs_export_clients()


def _run_setup_nfs_exports(lkp, mounts, tmp_path, existing=None, prepare_paths=True, bucket=None):
    exports_file = tmp_path / "exports.d" / "slurm.exports"
    exports_file.parent.mkdir(parents=True, exist_ok=True)
    if existing is not None:
        exports_file.write_text(existing)

    def is_ctrl(m):
        return bool(m) and not m.server_ip

    with patch("util.lookup", return_value=lkp), \
         patch("setup_network_storage.lookup", return_value=lkp), \
         patch("setup_network_storage.SLURM_EXPORTS_FILE", exports_file), \
         patch("setup_network_storage.resolve_network_storage", return_value=mounts), \
         patch("setup_network_storage.is_controller_mount", side_effect=is_ctrl), \
         patch("setup_network_storage.util.should_mount_slurm_bucket", return_value=(existing is not None) if bucket is None else bucket), \
         patch("setup_network_storage.get_slurm_bucket_mount", return_value=_mnt(server=None, path="/slurm/bucket", fs_type="gcsfuse")), \
         patch("setup_network_storage.util.mkdirp") as mock_mkdirp, \
         patch("setup_network_storage.os.chmod") as mock_chmod, \
         patch("setup_network_storage.run") as mock_run:
        setup_network_storage.setup_nfs_exports(prepare_paths=prepare_paths)
    mock_run.chmod = mock_chmod
    mock_run.mkdirp = mock_mkdirp
    return exports_file.read_text(), mock_run


def test_setup_nfs_exports_scopes_clients_and_options(tmp_path):
    lkp = _controller_lkp(nfs_export_clients=["10.0.0.0/24", "10.1.0.0/16"])
    mounts = [_mnt(server=None, path="/home"), _mnt(server=None, path="/opt/apps"), _mnt(path="/external")]
    text, mock_run = _run_setup_nfs_exports(lkp, mounts, tmp_path)

    assert "*" not in text
    lines = {l.split()[0]: l for l in text.splitlines() if l.strip()}
    assert set(lines) == {"/home", "/opt/apps", "/slurm/key_distribution"}
    assert lines["/home"] == "/home  10.0.0.0/24(rw,no_subtree_check,no_root_squash) 10.1.0.0/16(rw,no_subtree_check,no_root_squash)"
    assert "10.0.0.0/24(ro,no_subtree_check,no_root_squash)" in lines["/slurm/key_distribution"]
    assert "rw" not in lines["/slurm/key_distribution"]
    mock_run.assert_any_call("exportfs -ra", timeout=30)


def test_setup_nfs_exports_root_squash_opt_in(tmp_path):
    lkp = _controller_lkp(nfs_export_clients=["10.0.0.0/24"], nfs_export_root_squash=True)
    text, _ = _run_setup_nfs_exports(lkp, [_mnt(server=None, path="/home")], tmp_path)
    lines = {l.split()[0]: l for l in text.splitlines() if l.strip()}
    assert lines["/home"] == "/home  10.0.0.0/24(rw,no_subtree_check,root_squash)"
    # The key mount is always no_root_squash so nodes can copy the root-owned key.
    assert lines["/slurm/key_distribution"] == "/slurm/key_distribution  10.0.0.0/24(ro,no_subtree_check,no_root_squash)"


def test_setup_nfs_exports_reuses_bucket_fsid(tmp_path):
    lkp = _controller_lkp(nfs_export_clients=["10.0.0.0/24"])
    existing = "\n/slurm/bucket  *(ro,no_subtree_check,no_root_squash,fsid=abc-123)"
    text, _ = _run_setup_nfs_exports(lkp, [], tmp_path, existing=existing)
    lines = {l.split()[0]: l for l in text.splitlines() if l.strip()}
    assert lines["/slurm/bucket"] == "/slurm/bucket  10.0.0.0/24(ro,no_subtree_check,no_root_squash,fsid=abc-123)"


def test_refresh_does_not_touch_permissions_or_etc_exports(tmp_path):
    lkp = _controller_lkp(nfs_export_clients=["10.0.0.0/24", "10.99.0.0/24"])
    old = "\n/home  10.0.0.0/24(rw,no_subtree_check,no_root_squash)"
    mounts = [_mnt(server=None, path="/home"), _mnt(server=None, path="/scratch")]
    text, mock_run = _run_setup_nfs_exports(
        lkp, mounts, tmp_path, existing=old, prepare_paths=False, bucket=False)
    mock_run.mkdirp.assert_any_call(Path("/home"))
    mock_run.mkdirp.assert_any_call(Path("/scratch"))
    mock_run.chmod.assert_not_called()
    assert not any("sed" in str(c) for c in mock_run.call_args_list)
    assert "10.99.0.0/24(rw" in text
    assert "/scratch  10.0.0.0/24(rw,no_subtree_check,no_root_squash)" in text
    mock_run.assert_any_call("exportfs -ra", timeout=30)


def test_refresh_skips_exportfs_when_unchanged(tmp_path):
    lkp = _controller_lkp(nfs_export_clients=["10.0.0.0/24"])
    mounts = [_mnt(server=None, path="/home")]
    first, _ = _run_setup_nfs_exports(lkp, mounts, tmp_path, bucket=False)
    _, mock_run = _run_setup_nfs_exports(lkp, mounts, tmp_path, existing=first, prepare_paths=False, bucket=False)
    mock_run.assert_not_called()


def test_refresh_exports_requires_root():
    with patch("setup_network_storage.os.geteuid", return_value=1000), \
         patch("setup_network_storage.util._fetch_config") as mock_fetch, \
         patch("setup_network_storage.setup_nfs_exports") as mock_setup:
        with pytest.raises(SystemExit, match="must be run as root"):
            setup_network_storage.refresh_exports()
        mock_fetch.assert_not_called()
        mock_setup.assert_not_called()


def test_refresh_exports_fetches_from_gcs_without_writing_local_config(tmp_path):
    cfg_file = tmp_path / "config.yaml"
    hash_file = tmp_path / ".config.hash"
    cfg_file.write_text("nfs_export_clients: ['10.0.0.0/24']\n")
    hash_file.write_text("old-hash")
    new_cfg = NSDict(nfs_export_clients=["10.0.0.0/24", "10.8.0.0/24"])

    with patch("setup_network_storage.os.geteuid", return_value=0), \
         patch("setup_network_storage.util.CONFIG_FILE", cfg_file), \
         patch("setup_network_storage.util._fetch_config", return_value=(new_cfg, "new-hash")) as mock_fetch, \
         patch("setup_network_storage.util.update_config") as mock_update, \
         patch("setup_network_storage.setup_nfs_exports") as mock_setup:
        setup_network_storage.refresh_exports()

    mock_fetch.assert_called_once_with(old_hash=None)
    mock_update.assert_called_once_with(new_cfg)
    mock_setup.assert_called_once_with(prepare_paths=False)
    assert cfg_file.read_text() == "nfs_export_clients: ['10.0.0.0/24']\n"
    assert hash_file.read_text() == "old-hash"


def test_refresh_exports_dry_run_previews_and_warns_uncovered_clients(tmp_path, capsys, caplog):
    lkp = _controller_lkp(nfs_export_clients=["10.0.0.0/24"])
    exports_file = tmp_path / "exports.d" / "slurm.exports"
    ss_out = (
        "0  0  10.0.0.2:2049  10.0.0.4:751\n"
        "0  0  [::ffff:10.0.0.2]:2049  [::ffff:10.9.0.5]:832\n"
        "0  0  [::ffff:10.0.0.2]:2049  [::ffff:10.9.0.5]:833\n"
    )
    ss_res = MagicMock(returncode=0, stdout=ss_out)

    def is_ctrl(m):
        return bool(m) and not m.server_ip

    with patch("setup_network_storage.os.geteuid", return_value=0), \
         patch("setup_network_storage.util._fetch_config", return_value=None), \
         patch("util.lookup", return_value=lkp), \
         patch("setup_network_storage.SLURM_EXPORTS_FILE", exports_file), \
         patch("setup_network_storage.resolve_network_storage", return_value=[_mnt(server=None, path="/home")]), \
         patch("setup_network_storage.is_controller_mount", side_effect=is_ctrl), \
         patch("setup_network_storage.util.should_mount_slurm_bucket", return_value=False), \
         patch("setup_network_storage.run", return_value=ss_res) as mock_run:
        setup_network_storage.refresh_exports(dry_run=True)

    out = capsys.readouterr().out
    assert "/home  10.0.0.0/24(rw,no_subtree_check,no_root_squash)" in out
    assert not exports_file.exists()
    mock_run.assert_called_once_with("ss -Htn state established '( sport = :2049 )'", check=False, timeout=10)
    assert "10.9.0.5" in caplog.text
    assert "10.0.0.4" not in caplog.text
