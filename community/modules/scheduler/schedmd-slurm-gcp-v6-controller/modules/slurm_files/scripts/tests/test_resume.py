# Copyright 2024 "Google LLC"
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
      nodes=["c-n-0"], prefix="c-n", chunk_idx=0, excl_job_id=1, placement_group="g10"),
    "c-t:0": BulkChunk(
      nodes=["c-t-8", "c-t-9"], prefix="c-t", chunk_idx=0, excl_job_id=None, placement_group=None),
    "c-t:job2:0": BulkChunk(
      nodes=["c-t-0", "c-t-1"], prefix="c-t", chunk_idx=0, excl_job_id=2, placement_group=None),
    "c-t:job2:1": BulkChunk(
      nodes=["c-t-2", "c-t-3"], prefix="c-t", chunk_idx=1, excl_job_id=2, placement_group=None),
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
