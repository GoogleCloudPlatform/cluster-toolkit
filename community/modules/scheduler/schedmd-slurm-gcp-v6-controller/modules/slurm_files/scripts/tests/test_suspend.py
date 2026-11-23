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

from typing import Dict, Optional
from unittest import mock

from common import SOME_TS, TstCfg, TstNodeset
import suspend
import util


def _inst(name: str, zone: str = "us-central1-a", created_by: Optional[str] = None) -> util.Instance:
    meta: Dict[str, str] = {}
    if created_by is not None:
        meta["created-by"] = created_by
    return util.Instance(
        name=name,
        zone=zone,
        status="RUNNING",
        creation_timestamp=SOME_TS,
        role="compute",
        resource_status=util.InstanceResourceStatus.from_json(None),
        metadata=meta,
        scheduling=util.NSDict({}),
    )


def _lookup() -> util.Lookup:
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
    return util.Lookup(cfg)


@mock.patch("suspend.delete_instances")
@mock.patch("util.ensure_execute")
@mock.patch.object(util.Lookup, "compute", new_callable=mock.PropertyMock)
def test_suspend_mig_nodes_rollback_does_not_delete_standalone_vm(
    mock_compute_prop, mock_execute, mock_delete_instances
):
    lkp = _lookup()
    mock_compute = mock.MagicMock()
    mock_compute_prop.return_value = mock_compute
    mock_execute.side_effect = lambda req: req.execute()
    mock_compute.regionInstanceGroupManagers().listManagedInstances().execute.return_value = {
        "managedInstances": []
    }

    standalone = _inst("testcl-ns-0")
    with mock.patch.object(util.Lookup, "instance", return_value=standalone):
        # Default (retire_standalone=False, used by resume.py rollback) must NEVER touch standalone VMs
        suspend.suspend_mig_nodes(["testcl-ns-0"], lkp)

    mock_delete_instances.assert_not_called()
    mock_compute.regionInstanceGroupManagers().deleteInstances.assert_not_called()


@mock.patch("suspend.delete_instances")
@mock.patch("util.ensure_execute")
@mock.patch.object(util.Lookup, "compute", new_callable=mock.PropertyMock)
def test_suspend_mig_nodes_retires_legacy_standalone_and_respects_mig_ownership(
    mock_compute_prop, mock_execute, mock_delete_instances
):
    lkp = _lookup()
    mock_compute = mock.MagicMock()
    mock_compute_prop.return_value = mock_compute
    mock_execute.side_effect = lambda req: req.execute()
    # ns-0 is in the MIG; ns-1 (legacy standalone), ns-2 (same-MIG eventual-consistency lag),
    # ns-3 (owned by a different MIG) are all missing from listManagedInstances.
    mock_compute.regionInstanceGroupManagers().listManagedInstances().execute.return_value = {
        "managedInstances": [
            {"instance": "projects/testproj/zones/us-central1-a/instances/testcl-ns-0"},
        ]
    }
    gce_instances = {
        "testcl-ns-1": _inst("testcl-ns-1"),
        "testcl-ns-2": _inst(
            "testcl-ns-2",
            zone="us-central1-b",
            created_by="projects/123/regions/us-central1/instanceGroupManagers/testcl-ns-mig-0",
        ),
        "testcl-ns-3": _inst(
            "testcl-ns-3",
            zone="us-central1-c",
            created_by="projects/123/regions/us-central1/instanceGroupManagers/other-mig",
        ),
    }

    with mock.patch.object(util.Lookup, "instance", side_effect=lambda n: gce_instances.get(n)):
        suspend.suspend_mig_nodes(
            [
                "testcl-ns-0",
                "testcl-ns-1.c.testproj.internal",
                "testcl-ns-1",  # duplicate FQDN/short name must be deduped
                "testcl-ns-2",
                "testcl-ns-3",
            ],
            lkp,
            retire_standalone=True,
        )

    # ns-0 and ns-2 (same-MIG owner metadata) go through MIG deleteInstances
    del_calls = mock_compute.regionInstanceGroupManagers().deleteInstances.call_args_list
    assert len(del_calls) == 1
    assert del_calls[0].kwargs["body"]["instances"] == [
        "projects/testproj/zones/us-central1-a/instances/testcl-ns-0",
        "zones/us-central1-b/instances/testcl-ns-2",
    ]
    # Only ns-1 (true standalone legacy VM) goes to delete_instances, deduplicated
    mock_delete_instances.assert_called_once_with(["testcl-ns-1"], lkp=lkp)
    # PIC cleanup still runs across the shard
    assert mock_compute.regionInstanceGroupManagers().deletePerInstanceConfigs.called


@mock.patch("suspend.delete_instances", side_effect=RuntimeError("delete boom"))
@mock.patch("util.ensure_execute")
@mock.patch.object(util.Lookup, "compute", new_callable=mock.PropertyMock)
def test_suspend_mig_nodes_standalone_delete_error_does_not_skip_pic_cleanup(
    mock_compute_prop, mock_execute, mock_delete_instances
):
    lkp = _lookup()
    mock_compute = mock.MagicMock()
    mock_compute_prop.return_value = mock_compute
    mock_execute.side_effect = lambda req: req.execute()
    mock_compute.regionInstanceGroupManagers().listManagedInstances().execute.return_value = {
        "managedInstances": []
    }

    with mock.patch.object(util.Lookup, "instance", return_value=_inst("testcl-ns-0")):
        suspend.suspend_mig_nodes(["testcl-ns-0"], lkp, retire_standalone=True)

    mock_delete_instances.assert_called_once_with(["testcl-ns-0"], lkp=lkp)
    assert mock_compute.regionInstanceGroupManagers().deletePerInstanceConfigs.called


@mock.patch("suspend.suspend_mig_nodes")
@mock.patch("suspend.lookup")
def test_suspend_nodes_enables_retire_standalone_for_mig_nodes(mock_lookup, mock_suspend_mig):
    lkp = _lookup()
    mock_lookup.return_value = lkp

    suspend.suspend_nodes(["testcl-ns-0"])

    mock_suspend_mig.assert_called_once_with(["testcl-ns-0"], lkp, retire_standalone=True)


@mock.patch("suspend.delete_instances")
@mock.patch("util.ensure_execute")
@mock.patch.object(util.Lookup, "compute", new_callable=mock.PropertyMock)
def test_suspend_mig_nodes_instance_lookup_error_trips_circuit_breaker(
    mock_compute_prop, mock_execute, mock_delete_instances
):
    lkp = _lookup()
    mock_compute = mock.MagicMock()
    mock_compute_prop.return_value = mock_compute
    mock_execute.side_effect = lambda req: req.execute()
    mock_compute.regionInstanceGroupManagers().listManagedInstances().execute.return_value = {
        "managedInstances": []
    }

    with mock.patch.object(util.Lookup, "instance", side_effect=RuntimeError("503")) as mock_inst:
        suspend.suspend_mig_nodes(
            ["testcl-ns-0", "testcl-ns-1", "testcl-ns-2", "testcl-ns-3"],
            lkp,
            retire_standalone=True,
        )

    assert mock_inst.call_count == 1
    mock_delete_instances.assert_not_called()
