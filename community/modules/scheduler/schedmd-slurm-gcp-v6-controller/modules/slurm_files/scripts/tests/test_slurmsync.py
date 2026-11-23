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

from typing import Any, Dict, List, Optional
from unittest import mock

from common import SOME_TS, TstCfg, TstNodeset
import slurmsync
import util


def _make_instance(
    name: str,
    status: str = "RUNNING",
    preemptible: bool = False,
    metadata: Optional[Dict[str, str]] = None,
    zone: str = "us-central1-a",
) -> util.Instance:
    return util.Instance(
        name=name,
        zone=zone,
        status=status,
        creation_timestamp=SOME_TS,
        role="compute",
        resource_status=util.InstanceResourceStatus.from_json(None),
        metadata=metadata if metadata is not None else {},
        scheduling=util.NSDict({"preemptible": preemptible}),
    )


def _make_lookup(managed_instances: List[Dict[str, Any]]) -> tuple[util.Lookup, mock.MagicMock]:
    cfg = TstCfg(
        slurm_cluster_name="testcl",
        project="testproj",
        nodeset={
            "ns": TstNodeset(
                nodeset_name="ns",
                region="us-central1",
                provisioning_engine="MIG",
                node_count_static=4,
                node_count_dynamic_max=0,
            ),
        },
    )
    lkp = util.Lookup(cfg)
    mock_compute = mock.MagicMock()
    mock_compute.regionInstanceGroupManagers().listManagedInstances().execute.return_value = {
        "managedInstances": managed_instances,
    }
    return lkp, mock_compute


def test_instance_created_by_mig_helper():
    regional = _make_instance(
        "testcl-ns-0",
        metadata={
            "created-by": "projects/123/regions/us-central1/instanceGroupManagers/testcl-ns-mig-0"
        },
    )
    zonal = _make_instance(
        "testcl-ns-1",
        metadata={
            "created-by": "projects/123/zones/us-central1-a/instanceGroupManagers/testcl-ns-mig-0/"
        },
    )
    standalone = _make_instance("testcl-ns-2", metadata={"slurm_cluster_name": "testcl"})
    other = _make_instance("testcl-ns-3", metadata={"created-by": "manual-script"})

    assert util.instance_created_by_mig(regional) == "testcl-ns-mig-0"
    assert util.instance_created_by_mig(zonal) == "testcl-ns-mig-0"
    assert util.instance_created_by_mig(standalone) is None
    assert util.instance_created_by_mig(other) is None
    assert util.instance_created_by_mig(None) is None


def test_get_mig_instance_errors_filters_healthy_and_deleting():
    lkp, mock_compute = _make_lookup([
        {
            "name": "testcl-ns-0",
            "currentAction": "CREATING",
            "lastAttempt": {
                "errors": {
                    "errors": [
                        {"code": "QUOTA_EXCEEDED", "message": "Quota 'CPUS' exceeded\nin region."}
                    ]
                }
            },
        },
        {
            # Healthy slot carrying a leftover lastAttempt must be ignored
            "name": "testcl-ns-1",
            "currentAction": "NONE",
            "instanceStatus": "RUNNING",
            "lastAttempt": {
                "errors": {
                    "errors": [{"code": "ZONE_RESOURCE_POOL_EXHAUSTED", "message": "stale"}]
                }
            },
        },
        {
            # Deleting slot must be ignored
            "name": "testcl-ns-2",
            "currentAction": "DELETING",
            "lastAttempt": {
                "errors": {"errors": [{"code": "BUSY", "message": "ignore"}]}
            },
        },
        {
            # URL-only entry (no top-level "name")
            "instance": "projects/testproj/zones/us-central1-a/instances/testcl-ns-3",
            "currentAction": "RECREATING",
            "lastAttempt": {
                "errors": {
                    "errors": [{"code": "IP_SPACE_EXHAUSTED", "message": "No free IPs"}]
                }
            },
        },
    ])
    with mock.patch.object(
        util.Lookup, "compute", new_callable=mock.PropertyMock, return_value=mock_compute
    ):
        errs = lkp.get_mig_instance_errors("testproj", "us-central1", "testcl-ns-mig-0")

    assert errs == {
        "testcl-ns-0": ("CREATING", "QUOTA_EXCEEDED", "Quota 'CPUS' exceeded\nin region."),
        "testcl-ns-3": ("RECREATING", "IP_SPACE_EXHAUSTED", "No free IPs"),
    }


def test_format_mig_error_reason_sanitizes_brackets_and_truncates():
    reason = slurmsync._format_mig_error_reason(
        "CREATING",
        "QUOTA_EXCEEDED",
        "Line1\nLine2 [tag]\t" + "x" * 250,
    )
    assert "[" not in reason and "]" not in reason
    assert "\n" not in reason and "\t" not in reason
    assert reason.startswith("MIG Error (QUOTA_EXCEEDED): Line1 Line2 (tag) ")
    assert len(reason) <= slurmsync._MIG_REASON_MAX_LEN
    assert reason.endswith("...")

    repair_reason = slurmsync._format_mig_error_reason("RECREATING", "QUOTA_EXCEEDED", "")
    assert repair_reason == "MIG Auto-Healing failed (QUOTA_EXCEEDED)"


@mock.patch.object(util.Lookup, "instance", return_value=None)
@mock.patch.object(util.Lookup, "compute", new_callable=mock.PropertyMock)
@mock.patch("slurmsync.lookup")
def test_slurmsync_mig_error_surfaces_in_node_reason(mock_lookup, mock_compute_prop, _mock_inst):
    lkp, mock_compute = _make_lookup([
        {
            "name": "testcl-ns-0",
            "currentAction": "CREATING",
            "lastAttempt": {
                "errors": {
                    "errors": [
                        {
                            "code": "QUOTA_EXCEEDED",
                            "message": "Quota 'CPUS' exceeded in region [us-central1].",
                        }
                    ]
                }
            },
        },
        {
            "name": "testcl-ns-1",
            "currentAction": "CREATING",
            "lastAttempt": {
                "errors": {
                    "errors": [
                        {
                            "code": "ZONE_RESOURCE_POOL_EXHAUSTED",
                            "message": "The zone does not have enough resources.",
                        }
                    ]
                }
            },
        },
        {
            "name": "testcl-ns-2",
            "currentAction": "REPAIRING",
            "lastAttempt": {
                "errors": {
                    "errors": [
                        {
                            "code": "QUOTA_EXCEEDED",
                            "message": "Quota 'GPUS' exceeded.",
                        }
                    ]
                }
            },
        },
    ])
    mock_compute_prop.return_value = mock_compute
    mock_lookup.return_value = lkp

    # QUOTA_EXCEEDED during POWERING_UP -> mark DOWN immediately with specific GCE error
    powering_up = slurmsync.NodeState(base="IDLE", flags=frozenset(("POWERING_UP",)))
    with mock.patch.object(util.Lookup, "node_state", return_value=powering_up):
        act0 = slurmsync.get_node_action("testcl-ns-0")
        assert isinstance(act0, slurmsync.NodeActionDown)
        assert act0.reason == "MIG Error (QUOTA_EXCEEDED): Quota 'CPUS' exceeded in region (us-central1)."

        # Transient capacity exhaustion during POWERING_UP -> stay queued (no-op)
        act1 = slurmsync.get_node_action("testcl-ns-1")
        assert isinstance(act1, slurmsync.NodeActionUnchanged)

    # Once POWERING_UP clears, even ZONE_RESOURCE_POOL_EXHAUSTED surfaces in reason
    idle = slurmsync.NodeState(base="IDLE", flags=frozenset())
    with mock.patch.object(util.Lookup, "node_state", return_value=idle):
        act1_idle = slurmsync.get_node_action("testcl-ns-1")
        assert isinstance(act1_idle, slurmsync.NodeActionDown)
        assert "ZONE_RESOURCE_POOL_EXHAUSTED" in act1_idle.reason

    # Existing generic "MIG Auto-Healing instance repair in progress" upgrades to real error
    down = slurmsync.NodeState(base="DOWN", flags=frozenset())
    with mock.patch.object(util.Lookup, "node_state", return_value=down):
        with mock.patch("slurmsync.get_node_reason", return_value=slurmsync._MIG_REPAIRING_REASON):
            act2 = slurmsync.get_node_action("testcl-ns-2")
            assert isinstance(act2, slurmsync.NodeActionDown)
            assert act2.reason == "MIG Auto-Healing failed (QUOTA_EXCEEDED): Quota 'GPUS' exceeded."

        # Already holding the exact same error reason -> no redundant scontrol update and NOT powered down
        with mock.patch("slurmsync.get_node_reason", return_value=act2.reason):
            assert isinstance(slurmsync.get_node_action("testcl-ns-2"), slurmsync.NodeActionUnchanged)

        # Same for CREATING error: do NOT fall through to NodeActionPowerDown (which would wipe reason)
        with mock.patch("slurmsync.get_node_reason", return_value=act0.reason):
            assert isinstance(slurmsync.get_node_action("testcl-ns-0"), slurmsync.NodeActionUnchanged)

        # Operator's custom DOWN reason must NOT be overwritten
        with mock.patch("slurmsync.get_node_reason", return_value="bad DIMM replaced tomorrow"):
            assert isinstance(slurmsync.get_node_action("testcl-ns-0"), slurmsync.NodeActionUnchanged)
            assert isinstance(slurmsync.get_node_action("testcl-ns-2"), slurmsync.NodeActionUnchanged)

    # POWERED_DOWN (down~) clears via existing logic, never marked DOWN again for MIG error
    down_pd = slurmsync.NodeState(base="DOWN", flags=frozenset(("POWERED_DOWN",)))
    with mock.patch.object(util.Lookup, "node_state", return_value=down_pd):
        assert isinstance(slurmsync.get_node_action("testcl-ns-0"), slurmsync.NodeActionIdle)


@mock.patch.object(util.Lookup, "compute", new_callable=mock.PropertyMock)
@mock.patch("slurmsync.lookup")
def test_slurmsync_mig_error_auto_resumes_when_running(mock_lookup, mock_compute_prop):
    lkp, mock_compute = _make_lookup([
        {
            "name": "testcl-ns-0",
            "currentAction": "NONE",
            # Deliberately omit instanceStatus to confirm live inst.status == RUNNING still resumes
            "lastAttempt": {
                "errors": {"errors": [{"code": "QUOTA_EXCEEDED", "message": "stale"}]}
            },
        },
    ])
    mock_compute_prop.return_value = mock_compute
    mock_lookup.return_value = lkp

    running = _make_instance("testcl-ns-0", status="RUNNING")
    down = slurmsync.NodeState(base="DOWN", flags=frozenset())
    with mock.patch.object(util.Lookup, "instance", return_value=running):
        with mock.patch.object(util.Lookup, "node_state", return_value=down):
            with mock.patch(
                "slurmsync.get_node_reason",
                return_value="MIG Error (QUOTA_EXCEEDED): Quota 'CPUS' exceeded",
            ):
                assert isinstance(slurmsync.get_node_action("testcl-ns-0"), slurmsync.NodeActionIdle)

            with mock.patch(
                "slurmsync.get_node_reason",
                return_value="MIG Auto-Healing failed (QUOTA_EXCEEDED): Quota 'CPUS' exceeded",
            ):
                assert isinstance(slurmsync.get_node_action("testcl-ns-0"), slurmsync.NodeActionIdle)

            # Manual admin DOWN reason stays DOWN even when VM is RUNNING
            with mock.patch("slurmsync.get_node_reason", return_value="hardware maintenance"):
                assert isinstance(
                    slurmsync.get_node_action("testcl-ns-0"), slurmsync.NodeActionUnchanged
                )


@mock.patch.object(util.Lookup, "compute", new_callable=mock.PropertyMock)
@mock.patch("slurmsync.lookup")
def test_slurmsync_terminated_legacy_standalone_vm_uses_legacy_path(mock_lookup, mock_compute_prop):
    # MIG is empty (post-upgrade cluster with pre-existing standalone BULK_INSERT VMs)
    lkp, mock_compute = _make_lookup([])
    mock_compute_prop.return_value = mock_compute
    mock_lookup.return_value = lkp

    alloc = slurmsync.NodeState(base="ALLOCATED", flags=frozenset())

    # Legacy standalone Spot VM -> NodeActionPrempt (not stuck waiting on MIG auto-healing)
    spot_standalone = _make_instance("testcl-ns-0", status="TERMINATED", preemptible=True)
    with mock.patch.object(util.Lookup, "instance", return_value=spot_standalone):
        with mock.patch.object(util.Lookup, "node_state", return_value=alloc):
            assert isinstance(slurmsync.get_node_action("testcl-ns-0"), slurmsync.NodeActionPrempt)

    # Legacy standalone standard VM -> NodeActionDown("Instance terminated")
    std_standalone = _make_instance("testcl-ns-1", status="TERMINATED", preemptible=False)
    with mock.patch.object(util.Lookup, "instance", return_value=std_standalone):
        with mock.patch.object(util.Lookup, "node_state", return_value=alloc):
            action = slurmsync.get_node_action("testcl-ns-1")
            assert isinstance(action, slurmsync.NodeActionDown)
            assert action.reason == "Instance terminated"

    # VM carrying MIG created-by metadata (even if listManagedInstances lags and returns [])
    # MUST stay on MIG auto-healing wait path.
    mig_owned = _make_instance(
        "testcl-ns-2",
        status="TERMINATED",
        preemptible=True,
        metadata={
            "created-by": "projects/123/regions/us-central1/instanceGroupManagers/testcl-ns-mig-0"
        },
    )
    with mock.patch.object(util.Lookup, "instance", return_value=mig_owned):
        with mock.patch.object(util.Lookup, "node_state", return_value=alloc):
            action = slurmsync.get_node_action("testcl-ns-2")
            assert isinstance(action, slurmsync.NodeActionDown)
            assert action.reason == slurmsync._MIG_REPAIRING_REASON


@mock.patch.object(util.Lookup, "compute", new_callable=mock.PropertyMock)
@mock.patch("slurmsync.lookup")
def test_slurmsync_terminated_mig_list_error_fails_safe(mock_lookup, mock_compute_prop):
    lkp, mock_compute = _make_lookup([])
    mock_compute.regionInstanceGroupManagers().listManagedInstances().execute.side_effect = (
        RuntimeError("transient 503")
    )
    mock_compute_prop.return_value = mock_compute
    mock_lookup.return_value = lkp

    vm = _make_instance("testcl-ns-0", status="TERMINATED", preemptible=True)
    alloc = slurmsync.NodeState(base="ALLOCATED", flags=frozenset())
    with mock.patch.object(util.Lookup, "instance", return_value=vm):
        with mock.patch.object(util.Lookup, "node_state", return_value=alloc):
            action = slurmsync.get_node_action("testcl-ns-0")
            assert isinstance(action, slurmsync.NodeActionDown)
            assert action.reason == slurmsync._MIG_REPAIRING_REASON


def test_mig_listing_ignores_non_dict_entries():
    lkp, mock_compute = _make_lookup([
        None,  # type: ignore[list-item]
        "unexpected-entry",  # type: ignore[list-item]
        {"name": "testcl-ns-0", "currentAction": "REPAIRING"},
    ])
    with mock.patch.object(
        util.Lookup, "compute", new_callable=mock.PropertyMock, return_value=mock_compute
    ):
        assert lkp.get_mig_instance_names("testproj", "us-central1", "testcl-ns-mig-0") == {
            "testcl-ns-0"
        }
        assert lkp.get_mig_repairing_instances("testproj", "us-central1", "testcl-ns-mig-0") == {
            "testcl-ns-0"
        }
        assert lkp.get_mig_instance_errors("testproj", "us-central1", "testcl-ns-mig-0") == {}


def test_can_update_mig_reason_case_insensitive_and_preserves_manual():
    for reason in (
        None,
        "",
        "unbacked instance",
        "UNBACKED INSTANCE",
        "Instance Terminated",
        "Not responding",
        "Node unexpectedly rebooted",
        "MIG Auto-Healing instance repair in progress",
        "MIG Error (QUOTA_EXCEEDED): old",
    ):
        assert slurmsync._can_update_mig_reason(reason) is True

    for manual in ("bad DIMM replaced tomorrow", "admin drain for firmware", "HC failure"):
        assert slurmsync._can_update_mig_reason(manual) is False


@mock.patch("slurmsync.run")
@mock.patch("slurmsync.lookup")
def test_sync_instances_prefetches_node_reasons_and_clears_cache(mock_lookup, mock_run):
    lkp, mock_compute = _make_lookup([
        {
            "name": "testcl-ns-0",
            "currentAction": "CREATING",
            "lastAttempt": {
                "errors": {"errors": [{"code": "QUOTA_EXCEEDED", "message": "No CPUs"}]}
            },
        },
        {
            "name": "testcl-ns-1",
            "currentAction": "CREATING",
            "lastAttempt": {
                "errors": {"errors": [{"code": "QUOTA_EXCEEDED", "message": "No CPUs"}]}
            },
        },
    ])
    mock_lookup.return_value = lkp
    bulk_json = (
        '{"nodes": ['
        '{"name": "testcl-ns-0", "reason": "Not responding [slurm@2026-11-23T00:00:00]"},'
        '{"name": "testcl-ns-1", "reason": "admin maintenance [root@2026-11-23T00:00:00]"}'
        ']}'
    )
    mock_run.return_value = mock.MagicMock(stdout=bulk_json)
    down = slurmsync.NodeState(base="DOWN", flags=frozenset())

    with (
        mock.patch.object(util.Lookup, "compute", new_callable=mock.PropertyMock, return_value=mock_compute),
        mock.patch.object(util.Lookup, "instances", return_value={}),
        mock.patch.object(
            util.Lookup, "slurm_nodes", return_value={"testcl-ns-0": down, "testcl-ns-1": down}
        ),
        mock.patch.object(util.Lookup, "node_state", return_value=down),
    ):
        slurmsync.sync_instances()

    # Cache must always be cleared after sync_instances finishes
    assert slurmsync._node_reasons_cache is None
    cmds = [call.args[0] for call in mock_run.call_args_list]
    # Single bulk scontrol show nodes --json, NO per-node show node queries
    assert any("show nodes --json" in c for c in cmds)
    assert not any("show node " in c for c in cmds)
    # Only testcl-ns-0 updated; manual reason on testcl-ns-1 preserved
    down_cmds = [c for c in cmds if "state=down" in c]
    assert len(down_cmds) == 1
    assert "testcl-ns-0" in down_cmds[0]
    assert "testcl-ns-1" not in down_cmds[0]


@mock.patch("slurmsync.run")
@mock.patch("slurmsync.lookup")
def test_load_all_node_reasons_fallback_on_error(mock_lookup, mock_run):
    lkp, _ = _make_lookup([])
    mock_lookup.return_value = lkp
    mock_run.side_effect = [
        slurmsync.subprocess.CalledProcessError(1, "scontrol"),
        mock.MagicMock(stdout='{"nodes": [{"name": "testcl-ns-0", "reason": "Instance terminated [slurm@now]"}]}'),
    ]

    assert slurmsync._load_all_node_reasons() is None
    assert slurmsync.get_node_reason("testcl-ns-0") == "Instance terminated"


@mock.patch.object(util.Lookup, "compute", new_callable=mock.PropertyMock)
@mock.patch("slurmsync.lookup")
def test_mig_stale_error_ignored_while_vm_provisioning_or_staging(mock_lookup, mock_compute_prop):
    for status in ("PROVISIONING", "STAGING"):
        lkp, mock_compute = _make_lookup([
            {
                "name": "testcl-ns-0",
                "currentAction": "NONE",
                "instanceStatus": status,
                "lastAttempt": {
                    "errors": {"errors": [{"code": "QUOTA_EXCEEDED", "message": "stale"}]}
                },
            },
            {
                # instanceStatus omitted on slot, live instance lookup reports booting status
                "name": "testcl-ns-1",
                "currentAction": "NONE",
                "lastAttempt": {
                    "errors": {"errors": [{"code": "QUOTA_EXCEEDED", "message": "stale"}]}
                },
            },
        ])
        mock_compute_prop.return_value = mock_compute
        mock_lookup.return_value = lkp
        assert "testcl-ns-0" not in lkp.get_mig_instance_errors(
            "testproj", "us-central1", "testcl-ns-mig-0"
        )
        booting = _make_instance("testcl-ns-1", status=status)
        powering_up = slurmsync.NodeState(base="IDLE", flags=frozenset(("POWERING_UP",)))
        with (
            mock.patch.object(util.Lookup, "instance", return_value=booting),
            mock.patch.object(util.Lookup, "node_state", return_value=powering_up),
        ):
            assert isinstance(slurmsync.get_node_action("testcl-ns-1"), slurmsync.NodeActionUnchanged)
