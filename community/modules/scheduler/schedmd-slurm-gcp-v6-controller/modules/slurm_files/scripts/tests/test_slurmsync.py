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

from unittest.mock import Mock

from common import TstCfg, TstNodeset
import slurmsync
import util
from util import NodeState


def test_slurmsync_allocated_power_down_unbacked_instance(mocker):
    cfg = TstCfg(
        slurm_cluster_name="testcl",
        project="testproj",
        nodeset={
            "ns": TstNodeset(
                nodeset_name="ns", node_count_static=0, node_count_dynamic_max=10
            )
        },
    )
    lkp = util.Lookup(cfg)
    mocker.patch("slurmsync.lookup", return_value=lkp)
    mocker.patch.object(util.Lookup, "instance", return_value=None)

    # PowerDownOnIdle=YES sets POWER_DOWN on ALLOCATED/MIXED nodes (alloc!/mix!); deleted VM must immediately down node
    for active_base in ("ALLOCATED", "MIXED"):
        mocker.patch.object(
            util.Lookup,
            "node_state",
            return_value=NodeState(base=active_base, flags=frozenset({"CLOUD", "POWER_DOWN"})),
        )
        act = slurmsync.get_node_action("testcl-ns-0")
        assert isinstance(act, slurmsync.NodeActionDown)
        assert act.reason == "Unbacked instance"

    # IDLE+CLOUD+POWER_DOWN (idle!) during normal suspend must NOT be marked DOWN
    mocker.patch.object(
        util.Lookup,
        "node_state",
        return_value=NodeState(base="IDLE", flags=frozenset({"CLOUD", "POWER_DOWN"})),
    )
    assert isinstance(slurmsync.get_node_action("testcl-ns-0"), slurmsync.NodeActionUnchanged)

    # Next cycle: DOWN+CLOUD+POWER_DOWN (down!) must force power down to clear back to idle~
    mocker.patch.object(
        util.Lookup,
        "node_state",
        return_value=NodeState(base="DOWN", flags=frozenset({"CLOUD", "POWER_DOWN"})),
    )
    act2 = slurmsync.get_node_action("testcl-ns-0")
    assert isinstance(act2, slurmsync.NodeActionPowerDownForce)


def test_slurmsync_static_powered_down_resumes_and_powers_up(mocker):
    cfg = TstCfg(
        slurm_cluster_name="testcl",
        project="testproj",
        nodeset={
            "st": TstNodeset(
                nodeset_name="st", node_count_static=2, node_count_dynamic_max=0
            )
        },
    )
    lkp = util.Lookup(cfg)
    mocker.patch("slurmsync.lookup", return_value=lkp)
    mocker.patch.object(util.Lookup, "instance", return_value=None)
    mocker.patch.object(
        util.Lookup,
        "node_state",
        return_value=NodeState(base="DOWN", flags=frozenset({"CLOUD", "POWERED_DOWN"})),
    )

    act = slurmsync.get_node_action("testcl-st-0")
    assert isinstance(act, slurmsync.NodeActionStaticPowerUp)

    mock_run = mocker.patch("slurmsync.run")
    act.apply(["testcl-st-0", "testcl-st-1"])
    assert len(mock_run.call_args_list) == 2
    assert "state=resume" in mock_run.call_args_list[0].args[0]
    assert "state=power_up" in mock_run.call_args_list[1].args[0]


def test_sync_placement_groups_short_circuits_when_no_policies(mocker):
    lkp = Mock()
    mocker.patch("slurmsync.lookup", return_value=lkp)
    mocker.patch("slurmsync._get_resource_policies", return_value=[])
    slurmsync.sync_placement_groups()
    lkp.get_jobs.assert_not_called()


def test_get_node_action_uses_cached_reason_and_none_fallback(mocker):
    from types import SimpleNamespace

    cfg = TstCfg(
        slurm_cluster_name="testcl",
        project="testproj",
        nodeset={
            "ns": TstNodeset(
                nodeset_name="ns",
                region="us-central1",
                provisioning_engine="MIG",
                node_count_static=2,
                node_count_dynamic_max=0,
            )
        },
    )
    lkp = util.Lookup(cfg)
    mocker.patch("slurmsync.lookup", return_value=lkp)
    mocker.patch.object(util.Lookup, "get_mig_repairing_instances", return_value=set())
    mocker.patch.object(
        util.Lookup,
        "instance",
        return_value=SimpleNamespace(status="RUNNING"),
    )
    mock_get_reason = mocker.patch("slurmsync.get_node_reason", return_value="")

    # Cached empty/non-empty string in NodeState.reason avoids spawning scontrol subprocess
    mocker.patch.object(
        util.Lookup,
        "node_state",
        return_value=NodeState(base="DOWN", flags=frozenset({"CLOUD", "DRAIN"}), reason=""),
    )
    slurmsync.get_node_action("testcl-ns-0")
    mock_get_reason.assert_not_called()

    mocker.patch.object(
        util.Lookup,
        "node_state",
        return_value=NodeState(
            base="DOWN",
            flags=frozenset({"CLOUD"}),
            reason="MIG Auto-Healing instance repair in progress",
        ),
    )
    assert isinstance(slurmsync.get_node_action("testcl-ns-0"), slurmsync.NodeActionIdle)
    mock_get_reason.assert_not_called()

    # Default reason=None falls back to get_node_reason()
    mocker.patch.object(
        util.Lookup,
        "node_state",
        return_value=NodeState(base="DOWN", flags=frozenset({"CLOUD", "DRAIN"})),
    )
    slurmsync.get_node_action("testcl-ns-0")
    mock_get_reason.assert_called_with("testcl-ns-0")


def test_reconfigure_slurm_isolates_nfsd_refresh_errors(mocker):
    lkp = Mock()
    lkp.cfg.hybrid = False
    lkp.is_controller = True
    mocker.patch("slurmsync.lookup", return_value=lkp)
    mocker.patch("util.fetch_config", return_value=(True, Mock()))
    mocker.patch("util.update_config")
    mocker.patch("conf.get_generator", return_value=Mock())
    mocker.patch.object(slurmsync.Path, "exists", return_value=True)
    mocker.patch.object(slurmsync.Path, "read_text", side_effect=PermissionError("denied"))

    assert slurmsync.reconfigure_slurm() is True


def test_reconfigure_slurm_nfsd_hook_optional_and_invoked_when_present(mocker, monkeypatch):
    import setup_network_storage

    cfg = TstCfg(slurm_cluster_name="scalem1", hybrid=False)
    lkp = util.Lookup(cfg)
    mocker.patch("slurmsync.lookup", return_value=lkp)
    mocker.patch.object(util.Lookup, "is_controller", new=True)
    mocker.patch("util.fetch_config", return_value=(True, cfg))
    mocker.patch("util.update_config")
    mock_gen = mocker.patch("conf.get_generator")
    mocker.patch.object(slurmsync.Path, "exists", return_value=True)
    mocker.patch.object(slurmsync.Path, "read_text", return_value="/home *(rw)\n")

    # Safe no-op when configure_nfsd_threads is not yet present
    monkeypatch.delattr(setup_network_storage, "configure_nfsd_threads", raising=False)
    assert slurmsync.reconfigure_slurm() is True
    mock_gen.return_value.generate_configs.assert_called_once()

    # Invoked with lookup() when configure_nfsd_threads is available
    mock_cfg_nfsd = Mock()
    monkeypatch.setattr(setup_network_storage, "configure_nfsd_threads", mock_cfg_nfsd, raising=False)
    assert slurmsync.reconfigure_slurm() is True
    mock_cfg_nfsd.assert_called_once_with(lkp)
