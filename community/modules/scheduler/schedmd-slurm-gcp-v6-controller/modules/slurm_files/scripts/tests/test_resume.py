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

from typing import Optional

import os
import pytest
import unittest.mock
import unittest
import tempfile

from common import TstCfg, TstNodeset, TstPartition, TstTPU # needed to import util
import util
import error_handler
import resume
from resume import ResumeData, ResumeJobData, BulkChunk, PlacementAndNodes

def test_get_resume_file_data_no_env():
  with unittest.mock.patch.dict(os.environ, {"SLURM_RESUME_FILE": ""}):
    assert resume.get_resume_file_data() is None


def test_get_resume_file_data():
  with tempfile.NamedTemporaryFile() as f:
    f.write(b"""{
  "jobs": [
    {
      "extra": null,
      "job_id": 1,
      "features": null,
      "nodes_alloc": "green-[0-2]",
      "nodes_resume": "green-[0-1]",
      "oversubscribe": "OK",
      "partition": "red",
      "reservation": null
    }
  ],
  "all_nodes_resume": "green-[0-1]"
}""")
    f.flush()
    with (
      unittest.mock.patch.dict(os.environ, {"SLURM_RESUME_FILE": f.name}),
      unittest.mock.patch("util.to_hostnames") as mock_to_hostnames,
    ):
      mock_to_hostnames.return_value = ["green-0", "green-1", "green-2"]
      assert resume.get_resume_file_data() == ResumeData(jobs=[
        ResumeJobData(
          job_id = 1,
          partition="red",
          nodes_alloc=["green-0", "green-1", "green-2"],
        )
      ])
      mock_to_hostnames.assert_called_once_with("green-[0-2]")


@unittest.mock.patch("tpu.TPU.make")
@unittest.mock.patch("resume.create_placements")
def test_group_nodes_bulk(mock_create_placements, mock_tpu):
  cfg = TstCfg(
      slurm_cluster_name="c",
      nodeset={
        "n": TstNodeset(nodeset_name="n"),
      },
      nodeset_tpu={
        "t": TstNodeset(nodeset_name="t"),
      },
      partitions={
        "p1": TstPartition(
          partition_name="p1",
          enable_job_exclusive=True,
        ),
        "p2": TstPartition(
          partition_name="p2", 
          partition_nodeset_tpu=["t"],
          enable_job_exclusive=True,
        )
      }
  )
  lkp = util.Lookup(cfg)

  def mock_create_placements_se(nodes, excl_job_id, lkp):
    args = (set(nodes), excl_job_id)
    if ({'c-n-1', 'c-n-2', 'c-t-8', 'c-t-9'}, None) == args:
      return [
        PlacementAndNodes("g0", ["c-n-1", "c-n-2"]),
        PlacementAndNodes(None, ['c-t-8', 'c-t-9']),
      ]
    if ({"c-n-0", "c-n-8"}, 1) == args:
      return [
        PlacementAndNodes("g10", ["c-n-0"]),
        PlacementAndNodes("g11", ["c-n-8"]), 
      ]
    if ({'c-t-0', 'c-t-1', 'c-t-2', 'c-t-3', 'c-t-4', 'c-t-5'}, 2) == args:
      return [
        PlacementAndNodes(None, ['c-t-0', 'c-t-1', 'c-t-2', 'c-t-3', 'c-t-4', 'c-t-5'])
      ]
    raise AssertionError(f"unexpected invocation: '{args}'")
  mock_create_placements.side_effect = mock_create_placements_se

  def mock_tpu_se(ns: str, lkp) -> TstTPU:
    if ns == "t":
      return TstTPU(vmcount=2)
    raise AssertionError(f"unexpected invocation: '{ns}'")
  mock_tpu.side_effect = mock_tpu_se

  got = resume.group_nodes_bulk(
    ["c-n-0", "c-n-1", "c-n-2", "c-t-0", "c-t-1", "c-t-2", "c-t-3", "c-t-8", "c-t-9"], 
    ResumeData(jobs=[
      ResumeJobData(job_id=1, partition="p1", nodes_alloc=["c-n-0", "c-n-8"]),
      ResumeJobData(job_id=2, partition="p2", nodes_alloc=["c-t-0", "c-t-1", "c-t-2", "c-t-3", "c-t-4", "c-t-5"]),
    ]), lkp)
  mock_create_placements.assert_called()
  assert got == {
    "c-n:jobNone:g0:0": BulkChunk(
      nodes=["c-n-1", "c-n-2"], prefix="c-n", chunk_idx=0, excl_job_id=None, placement_group="g0"),
    "c-n:job1:g10:0": BulkChunk(
      nodes=["c-n-0"], prefix="c-n", chunk_idx=0, excl_job_id=1, placement_group="g10", is_job_request=True),
    "c-t:0": BulkChunk(
      nodes=["c-t-8", "c-t-9"], prefix="c-t", chunk_idx=0, excl_job_id=None, placement_group=None),
    "c-t:job2:0": BulkChunk(
      nodes=["c-t-0", "c-t-1"], prefix="c-t", chunk_idx=0, excl_job_id=2, placement_group=None, is_job_request=True),
    "c-t:job2:1": BulkChunk(
      nodes=["c-t-2", "c-t-3"], prefix="c-t", chunk_idx=1, excl_job_id=2, placement_group=None, is_job_request=True),
  }


@pytest.mark.parametrize(
    "nodes,excl_job_id,expected",
    [
        ( # TPU - no placements
          ["c-t-0", "c-t-2"], 4, [PlacementAndNodes(None, ["c-t-0", "c-t-2"])]
        ),
        ( # disabled placements - no placemens
          ["c-x-0", "c-x-2"], 4, [PlacementAndNodes(None, ["c-x-0", "c-x-2"])]
        ),
        ( # excl_job
          ["c-n-0", "c-n-uno", "c-n-2", "c-n-2011"], 4, [
            PlacementAndNodes("c-slurmgcp-managed-n-4-0", ["c-n-0", "c-n-uno", "c-n-2", "c-n-2011"])
          ]
        ),
        ( # no excl_job
          ["c-n-0", "c-n-uno", "c-n-2", "c-n-2011"], None, [
            PlacementAndNodes("c-slurmgcp-managed-n-0-0", ["c-n-0", "c-n-2"]),
            PlacementAndNodes('c-slurmgcp-managed-n-0-1', ['c-n-2011']),
            PlacementAndNodes(None, ["c-n-uno"]),
          ]
        ),
    ],
)
def test_allocate_nodes_to_placements(nodes: list[str], excl_job_id: Optional[int], expected: list[PlacementAndNodes]):
  cfg = TstCfg(
      slurm_cluster_name="c",
      nodeset={
        "n": TstNodeset(nodeset_name="n", enable_placement=True),
        "x": TstNodeset(nodeset_name="x", enable_placement=False)
      },
      nodeset_tpu={
        "t": TstNodeset(nodeset_name="t")
      })
  lkp = util.Lookup(cfg)

  with unittest.mock.patch("resume.valid_placement_node") as mock_valid_placement_node:
    mock_valid_placement_node.return_value = True
    lkp.template_info = unittest.mock.Mock(return_value=unittest.mock.Mock(machine_type=unittest.mock.Mock(family="n1")))

    assert resume._allocate_nodes_to_placements(nodes, excl_job_id, lkp) == expected


@unittest.mock.patch("resume.ensure_execute")
@unittest.mock.patch.object(util.Lookup, "compute", new_callable=unittest.mock.PropertyMock)
def test_resume_mig_nodes(mock_compute_prop, mock_execute):
  cfg = TstCfg(
      slurm_cluster_name="c",
      provisioning_engine="MIG",
      nodeset={
          "n": TstNodeset(nodeset_name="n"),
      }
  )
  lkp = util.Lookup(cfg)
  mock_compute = unittest.mock.MagicMock()
  mock_compute_prop.return_value = mock_compute

  # Pass an FQDN to verify instance name normalization
  resume.resume_mig_nodes(["c-n-0.c.testproj.internal", "c-n-1"], excl_job_id=101, lkp=lkp)

  assert mock_compute.regionInstanceGroupManagers().createInstances.called
  call_args = mock_compute.regionInstanceGroupManagers().createInstances.call_args
  assert call_args.kwargs["instanceGroupManager"] == "c-n-mig-0"
  assert call_args.kwargs["body"]["instances"] == [
      {"name": "c-n-0", "preservedState": {"metadata": {"slurm_job_id": "101"}}},
      {"name": "c-n-1", "preservedState": {"metadata": {"slurm_job_id": "101"}}},
  ]


@unittest.mock.patch("resume.ensure_execute")
@unittest.mock.patch.object(util.Lookup, "compute", new_callable=unittest.mock.PropertyMock)
def test_resume_mig_nodes_drift_update(mock_compute_prop, mock_execute):
  cfg = TstCfg(
      slurm_cluster_name="c",
      provisioning_engine="MIG",
      project="p",
      nodeset={
          "n": TstNodeset(nodeset_name="n", region="r", instance_template="projects/p/global/instanceTemplates/t2"),
      }
  )
  lkp = util.Lookup(cfg)
  mock_compute = unittest.mock.MagicMock()
  mock_compute_prop.return_value = mock_compute
  mock_compute.regionInstanceGroupManagers().get().execute.return_value = {
      "instanceTemplate": "projects/p/global/instanceTemplates/t1"
  }

  resume.resume_mig_nodes(["c-n-0"], excl_job_id=None, lkp=lkp)

  assert mock_compute.regionInstanceGroupManagers().patch.called
  patch_call = mock_compute.regionInstanceGroupManagers().patch.call_args
  assert patch_call.kwargs["instanceGroupManager"] == "c-n-mig-0"
  assert patch_call.kwargs["body"]["versions"][0]["instanceTemplate"] == "projects/p/global/instanceTemplates/t2"


@unittest.mock.patch("resume.ensure_execute")
@unittest.mock.patch.object(util.Lookup, "compute", new_callable=unittest.mock.PropertyMock)
def test_resume_mig_nodes_drift_update_preserves_version_name(mock_compute_prop, mock_execute):
  cfg = TstCfg(
      slurm_cluster_name="c",
      provisioning_engine="MIG",
      project="p",
      nodeset={
          "n": TstNodeset(nodeset_name="n", region="r", instance_template="t2"),
      }
  )
  lkp = util.Lookup(cfg)
  mock_compute = unittest.mock.MagicMock()
  mock_compute_prop.return_value = mock_compute
  mock_compute.regionInstanceGroupManagers().get().execute.return_value = {
      "versions": [{"name": "primary-v1", "instanceTemplate": "t1"}]
  }

  resume.resume_mig_nodes(["c-n-0"], excl_job_id=None, lkp=lkp)

  assert mock_compute.regionInstanceGroupManagers().patch.called
  patch_call = mock_compute.regionInstanceGroupManagers().patch.call_args
  assert patch_call.kwargs["instanceGroupManager"] == "c-n-mig-0"
  assert patch_call.kwargs["body"]["versions"][0]["instanceTemplate"] == "t2"
  assert patch_call.kwargs["body"]["versions"][0]["name"] == "primary-v1"


@unittest.mock.patch("resume.ensure_execute")
@unittest.mock.patch.object(util.Lookup, "compute", new_callable=unittest.mock.PropertyMock)
def test_resume_mig_nodes_no_drift(mock_compute_prop, mock_execute):
  cfg = TstCfg(
      slurm_cluster_name="c",
      provisioning_engine="MIG",
      project="p",
      nodeset={
          "n": TstNodeset(nodeset_name="n", region="r", instance_template="projects/p/global/instanceTemplates/t1"),
      }
  )
  lkp = util.Lookup(cfg)
  mock_compute = unittest.mock.MagicMock()
  mock_compute_prop.return_value = mock_compute
  mock_compute.regionInstanceGroupManagers().get().execute.return_value = {
      "instanceTemplate": "projects/p/global/instanceTemplates/t1"
  }

  resume.resume_mig_nodes(["c-n-0"], excl_job_id=None, lkp=lkp)

  assert not mock_compute.regionInstanceGroupManagers().patch.called


@unittest.mock.patch("resume.ensure_execute")
@unittest.mock.patch.object(util.Lookup, "compute", new_callable=unittest.mock.PropertyMock)
def test_resume_mig_nodes_chunking(mock_compute_prop, mock_execute):
  cfg = TstCfg(
      slurm_cluster_name="c",
      provisioning_engine="MIG",
      nodeset={
          "n": TstNodeset(nodeset_name="n", node_count_static=1200, instance_template="projects/p/global/instanceTemplates/t1"),
      }
  )
  lkp = util.Lookup(cfg)
  mock_compute = unittest.mock.MagicMock()
  mock_compute_prop.return_value = mock_compute

  # Resume 1200 nodes
  large_nodes = [f"c-n-{i}" for i in range(1200)]
  resume.resume_mig_nodes(large_nodes, excl_job_id=None, lkp=lkp)

  # Verify createInstances was called for each instance group
  create_calls = mock_compute.regionInstanceGroupManagers().createInstances.call_args_list
  assert len(create_calls) == 2
  assert create_calls[0].kwargs["instanceGroupManager"] == "c-n-mig-0"
  assert len(create_calls[0].kwargs["body"]["instances"]) == 1000
  assert create_calls[1].kwargs["instanceGroupManager"] == "c-n-mig-1"
  assert len(create_calls[1].kwargs["body"]["instances"]) == 200


@unittest.mock.patch("resume.handle_resume_failure")
@unittest.mock.patch.object(util.Lookup, "compute", new_callable=unittest.mock.PropertyMock)
def test_resume_mig_nodes_exception_handling(mock_compute_prop, mock_handle_failure):
  cfg = TstCfg(
      slurm_cluster_name="c",
      provisioning_engine="MIG",
      nodeset={
          "n": TstNodeset(nodeset_name="n", node_count_static=2, instance_template="projects/p/global/instanceTemplates/t1"),
      }
  )
  lkp = util.Lookup(cfg)
  mock_compute = unittest.mock.MagicMock()
  mock_compute_prop.return_value = mock_compute
  mock_compute.regionInstanceGroupManagers().createInstances.side_effect = Exception("Quota Exceeded")

  resume.resume_mig_nodes(["c-n-0", "c-n-1"], excl_job_id=None, lkp=lkp)

  mock_handle_failure.assert_called_once()
  assert mock_handle_failure.call_args[0][0] == ["c-n-0", "c-n-1"]
  assert "Quota Exceeded" in mock_handle_failure.call_args[0][1]


@unittest.mock.patch("resume.ensure_execute")
@unittest.mock.patch.object(util.Lookup, "compute", new_callable=unittest.mock.PropertyMock)
def test_resume_mig_nodes_multi_mig_shard_continuity(mock_compute_prop, mock_execute):
  """Verifies that if shard 0 has all instances existing, shard 1 is still processed (continue, not return)."""
  cfg = TstCfg(
      slurm_cluster_name="c",
      provisioning_engine="MIG",
      nodeset={
          "n": TstNodeset(nodeset_name="n", node_count_static=1200, instance_template="projects/p/global/instanceTemplates/t1"),
      }
  )
  lkp = util.Lookup(cfg)
  mock_compute = unittest.mock.MagicMock()
  mock_compute_prop.return_value = mock_compute

  # Shard 0 has c-n-0 existing; Shard 1 has no instances
  def mock_list_instances(project, region, instanceGroupManager, **kwargs):
    mock_req = unittest.mock.MagicMock()
    if instanceGroupManager == "c-n-mig-0":
      mock_req.execute.return_value = {"managedInstances": [{"name": "c-n-0"}]}
    else:
      mock_req.execute.return_value = {"managedInstances": []}
    return mock_req

  mock_compute.regionInstanceGroupManagers().listManagedInstances.side_effect = mock_list_instances

  # Resume 1 node from shard 0 and 1 node from shard 1
  resume.resume_mig_nodes(["c-n-0", "c-n-1001"], excl_job_id=None, lkp=lkp)

  # Shard 0 (c-n-mig-0) had all instances existing -> skipped
  # Shard 1 (c-n-mig-1) must still have createInstances called!
  create_calls = mock_compute.regionInstanceGroupManagers().createInstances.call_args_list
  assert len(create_calls) == 1
  assert create_calls[0].kwargs["instanceGroupManager"] == "c-n-mig-1"
  assert create_calls[0].kwargs["body"]["instances"] == [{"name": "c-n-1001"}]


@unittest.mock.patch("resume.ensure_execute")
@unittest.mock.patch.object(util.Lookup, "compute", new_callable=unittest.mock.PropertyMock)
def test_resume_mig_nodes_deduplication(mock_compute_prop, mock_execute):
  """Verifies that duplicate entries in requested nodes list are filtered out before createInstances."""
  cfg = TstCfg(
      slurm_cluster_name="c",
      provisioning_engine="MIG",
      nodeset={
          "n": TstNodeset(nodeset_name="n", node_count_static=2, instance_template="projects/p/global/instanceTemplates/t1"),
      }
  )
  lkp = util.Lookup(cfg)
  mock_compute = unittest.mock.MagicMock()
  mock_compute_prop.return_value = mock_compute
  mock_compute.regionInstanceGroupManagers().listManagedInstances().execute.return_value = {"managedInstances": []}

  resume.resume_mig_nodes(["c-n-0", "c-n-0", "c-n-1", "c-n-1"], excl_job_id=None, lkp=lkp)

  create_calls = mock_compute.regionInstanceGroupManagers().createInstances.call_args_list
  assert len(create_calls) == 1
  assert create_calls[0].kwargs["body"]["instances"] == [{"name": "c-n-0"}, {"name": "c-n-1"}]


@unittest.mock.patch("resume.handle_resume_failure")
@unittest.mock.patch("resume.wait_for_operation")
@unittest.mock.patch("resume.ensure_execute")
@unittest.mock.patch.object(util.Lookup, "compute", new_callable=unittest.mock.PropertyMock)
def test_resume_mig_nodes_async_operation_error(mock_compute_prop, mock_execute, mock_wait_op, mock_handle_failure):
  """Verifies that when createInstances returns an operation that completes with error, resume_mig_nodes raises and calls handle_resume_failure."""
  cfg = TstCfg(
      slurm_cluster_name="c",
      provisioning_engine="MIG",
      nodeset={
          "n": TstNodeset(nodeset_name="n", node_count_static=2, instance_template="projects/p/global/instanceTemplates/t1"),
      }
  )
  lkp = util.Lookup(cfg)
  mock_compute = unittest.mock.MagicMock()
  mock_compute_prop.return_value = mock_compute

  # ensure_execute returns operation object with selfLink
  mock_execute.return_value = {
      "name": "op-123",
      "selfLink": "https://.../operations/op-123",
      "status": "RUNNING",
  }

  # wait_for_operation returns operation completed with error
  mock_wait_op.return_value = {
      "status": "DONE",
      "error": {
          "errors": [{
              "code": "ZONE_RESOURCE_POOL_EXHAUSTED",
              "message": "Stockout in zone",
          }]
      }
  }

  resume.resume_mig_nodes(["c-n-0", "c-n-1"], excl_job_id=None, lkp=lkp)

  mock_handle_failure.assert_called_once()
  assert mock_handle_failure.call_args[0][0] == ["c-n-0", "c-n-1"]
  assert "ZONE_RESOURCE_POOL_EXHAUSTED" in mock_handle_failure.call_args[0][1]


@unittest.mock.patch("time.sleep")
@unittest.mock.patch("resume.handle_resume_failure")
@unittest.mock.patch.object(util.Lookup, "compute", new_callable=unittest.mock.PropertyMock)
def test_resume_mig_nodes_inflight_deletion_timeout(mock_compute_prop, mock_handle_failure, mock_sleep):
  """Verifies that if an instance remains deleting after polling timeout, handle_resume_failure is called with Action.REQUEUE."""
  cfg = TstCfg(
      slurm_cluster_name="c",
      provisioning_engine="MIG",
      nodeset={
          "n": TstNodeset(nodeset_name="n", node_count_static=1, instance_template="projects/p/global/instanceTemplates/t1"),
      }
  )
  lkp = util.Lookup(cfg)
  mock_compute = unittest.mock.MagicMock()
  mock_compute_prop.return_value = mock_compute

  # Instance is deleting and never finishes deleting during the timeout
  mock_compute.regionInstanceGroupManagers().listManagedInstances().execute.return_value = {
      "managedInstances": [{
          "instance": "https://www.googleapis.com/compute/v1/projects/p/zones/z/instances/c-n-0",
          "name": "c-n-0",
          "currentAction": "DELETING",
          "instanceStatus": "STOPPING",
      }]
  }

  resume.resume_mig_nodes(["c-n-0"], excl_job_id=None, lkp=lkp)

  mock_handle_failure.assert_called_once()
  call_args = mock_handle_failure.call_args
  assert call_args[0][0] == ["c-n-0"]
  assert call_args[0][3] == error_handler.Action.REQUEUE
  # Verify createInstances was never called for this node
  assert not mock_compute.regionInstanceGroupManagers().createInstances.called


@unittest.mock.patch("time.sleep")
@unittest.mock.patch("resume.handle_resume_failure")
@unittest.mock.patch.object(util.Lookup, "compute", new_callable=unittest.mock.PropertyMock)
def test_resume_mig_nodes_inflight_deletion_timeout_multinode_exclusive(mock_compute_prop, mock_handle_failure, mock_sleep):
  """Verifies that for exclusive multi-node jobs, if any node times out deleting, all nodes in the job are failed and createInstances is aborted."""
  cfg = TstCfg(
      slurm_cluster_name="c",
      provisioning_engine="MIG",
      nodeset={
          "n": TstNodeset(nodeset_name="n", node_count_static=2, instance_template="projects/p/global/instanceTemplates/t1"),
      }
  )
  lkp = util.Lookup(cfg)
  mock_compute = unittest.mock.MagicMock()
  mock_compute_prop.return_value = mock_compute

  # c-n-0 is deleting, c-n-1 is ready/not in MIG
  mock_compute.regionInstanceGroupManagers().listManagedInstances().execute.return_value = {
      "managedInstances": [{
          "instance": "https://www.googleapis.com/compute/v1/projects/p/zones/z/instances/c-n-0",
          "name": "c-n-0",
          "currentAction": "DELETING",
          "instanceStatus": "STOPPING",
      }]
  }

  resume.resume_mig_nodes(["c-n-0", "c-n-1"], excl_job_id=101, lkp=lkp)

  mock_handle_failure.assert_called_once()
  call_args = mock_handle_failure.call_args
  # Entire allocation failed to preserve atomicity and avoid orphan VMs
  assert call_args[0][0] == ["c-n-0", "c-n-1"]
  assert call_args[0][3] == error_handler.Action.REQUEUE
  # createInstances should NOT be called for c-n-1
  assert not mock_compute.regionInstanceGroupManagers().createInstances.called


@unittest.mock.patch("time.sleep")
@unittest.mock.patch("resume.handle_resume_failure")
@unittest.mock.patch.object(util.Lookup, "compute", new_callable=unittest.mock.PropertyMock)
def test_resume_mig_nodes_inflight_deletion_timeout_multishard_exclusive(mock_compute_prop, mock_handle_failure, mock_sleep):
  """Verifies that for multi-shard exclusive jobs, if shard 0 times out deleting, all shards are aborted and all nodes failed."""
  cfg = TstCfg(
      slurm_cluster_name="c",
      provisioning_engine="MIG",
      nodeset={
          "n": TstNodeset(nodeset_name="n", node_count_static=2000, instance_template="projects/p/global/instanceTemplates/t1"),
      }
  )
  lkp = util.Lookup(cfg)
  mock_compute = unittest.mock.MagicMock()
  mock_compute_prop.return_value = mock_compute

  # c-n-0 on Shard 0 is DELETING; c-n-1001 is on Shard 1
  def mock_list_instances(project, region, instanceGroupManager, **kwargs):
    mock_req = unittest.mock.MagicMock()
    if instanceGroupManager == "c-n-mig-0":
      mock_req.execute.return_value = {
          "managedInstances": [{
              "instance": "https://www.googleapis.com/compute/v1/projects/p/zones/z/instances/c-n-0",
              "name": "c-n-0",
              "currentAction": "DELETING",
              "instanceStatus": "STOPPING",
          }]
      }
    else:
      mock_req.execute.return_value = {"managedInstances": []}
    return mock_req

  mock_compute.regionInstanceGroupManagers().listManagedInstances.side_effect = mock_list_instances

  resume.resume_mig_nodes(["c-n-0", "c-n-1001"], excl_job_id=101, lkp=lkp)

  mock_handle_failure.assert_called_once()
  call_args = mock_handle_failure.call_args
  # All nodes across all shards must be reported to handle_resume_failure
  assert call_args[0][0] == ["c-n-0", "c-n-1001"]
  assert call_args[0][3] == error_handler.Action.REQUEUE
  # Shard 1 must NOT be called for createInstances
  assert not mock_compute.regionInstanceGroupManagers().createInstances.called


@unittest.mock.patch("resume.handle_resume_failure")
@unittest.mock.patch("resume.ensure_execute")
@unittest.mock.patch.object(util.Lookup, "compute", new_callable=unittest.mock.PropertyMock)
def test_resume_mig_nodes_stockout_multishard_exclusive(mock_compute_prop, mock_execute, mock_handle_failure):
  """Verifies that for multi-shard exclusive jobs, if shard 0 fails createInstances, all shards are aborted and all nodes failed."""
  cfg = TstCfg(
      slurm_cluster_name="c",
      provisioning_engine="MIG",
      nodeset={
          "n": TstNodeset(nodeset_name="n", node_count_static=2000, instance_template="projects/p/global/instanceTemplates/t1"),
      }
  )
  lkp = util.Lookup(cfg)
  mock_compute = unittest.mock.MagicMock()
  mock_compute_prop.return_value = mock_compute

  # Both shards have empty managed instances
  mock_compute.regionInstanceGroupManagers().listManagedInstances().execute.return_value = {"managedInstances": []}

  # First createInstances call (Shard 0) fails with stockout exception
  from googleapiclient.errors import HttpError  # type: ignore
  import httplib2
  resp = httplib2.Response({"status": 503})
  stockout_err = HttpError(resp, b"ZONE_RESOURCE_POOL_EXHAUSTED")
  mock_execute.side_effect = stockout_err

  resume.resume_mig_nodes(["c-n-0", "c-n-1001"], excl_job_id=101, lkp=lkp)

  mock_handle_failure.assert_called_once()
  call_args = mock_handle_failure.call_args
  # All nodes across all shards must be reported to handle_resume_failure
  assert call_args[0][0] == ["c-n-0", "c-n-1001"]
  # createInstances should only have been attempted once (Shard 0), Shard 1 must be aborted
  create_calls = mock_compute.regionInstanceGroupManagers().createInstances.call_args_list
  assert len(create_calls) == 1
  assert create_calls[0].kwargs["instanceGroupManager"] == "c-n-mig-0"
