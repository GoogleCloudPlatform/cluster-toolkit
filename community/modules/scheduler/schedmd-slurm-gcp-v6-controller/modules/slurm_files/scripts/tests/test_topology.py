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

import pytest
import json
import yaml
import mock
from pytest_unordered import unordered
from common import TstCfg, TstNodeset, TstTPU, tstInstance
import sort_nodes

import util
import conf
import tempfile
from pathlib import Path
import conf_v2411
import re
import uuid

PRELUDE = """
# Warning:
# This file is managed by a script. Manual modifications will be overwritten.
"""

BLOCK_SIZE = 32
NVLINK_VM_COUNT = 18

def test_gen_topology_conf_empty():
    out_dir = tempfile.mkdtemp()
    cfg = TstCfg(output_dir=out_dir)
    conf.gen_topology_yaml(util.Lookup(cfg))
    assert open(out_dir + "/cloud_topology.yaml").read() == PRELUDE + """---

- topology: topology/tree
  cluster_default: true
  tree:
    switches: []
"""


@mock.patch("tpu.TPU.make")
@mock.patch('uuid.uuid4')
def test_gen_topology_conf(mock_uuid, tpu_mock):
    mock_uuid.side_effect = [
        mock.MagicMock(hex=f'{i:05d}' + '0' * 27) for i in range(15)
    ]
    output_dir = tempfile.mkdtemp()
    cfg = TstCfg(
        nodeset_tpu={
            "a": TstNodeset("bold", node_count_static=4, node_count_dynamic_max=5),
            "b": TstNodeset("slim", node_count_dynamic_max=3),
        },
        nodeset={
            "c": TstNodeset("green", node_count_static=2, node_count_dynamic_max=3),
            "d": TstNodeset("blue", node_count_static=7),
            "e": TstNodeset("pink", node_count_dynamic_max=4),
        },
        output_dir=output_dir,
    )

    def tpu_se(ns: str, lkp) -> TstTPU:
        if ns == "bold":
            return TstTPU(vmcount=3)
        if ns == "slim":
            return TstTPU(vmcount=1)
        raise AssertionError(f"unexpected TPU name: '{ns}'")

    tpu_mock.side_effect = tpu_se

    lkp = util.Lookup(cfg)
    lkp.instances = lambda: { n.name: n for n in [ # type: ignore[assignment]
        # nodeset blue
        tstInstance("m22-blue-0"),  # no physicalHost
        tstInstance("m22-blue-0", physical_host="/a/a/a"),
        tstInstance("m22-blue-1", physical_host="/a/a/b"),
        tstInstance("m22-blue-2", physical_host="/a/b/a"),
        tstInstance("m22-blue-3", physical_host="/b/a/a"),
        # nodeset green
        tstInstance("m22-green-3", physical_host="/a/a/c"),
    ]}

    upd, summary = conf.gen_topology_yaml(lkp)
    assert upd == True
    
    yaml_file = Path(output_dir) / "cloud_topology.yaml"
    assert yaml_file.exists()
    yaml_content = yaml_file.read_text()
    
    parsed_yaml = yaml.safe_load(yaml_content)
    
    # Assert topology/tree section
    tree_topology = parsed_yaml[0]
    assert tree_topology['topology'] == 'topology/tree'
    assert tree_topology['cluster_default'] == True
    
    # Compare tree switches by loading the expected YAML for the tree part only
    # This avoids reproducing the very long string and focuses on comparing the structure
    expected_tree_yaml = PRELUDE + """---

- topology: topology/tree
  cluster_default: true
  tree:
    switches:
    - switch: s0
      children: s0_[0-4]
    - switch: s0_0
      children: s0_0_[0-1]
    - switch: s0_0_0
      nodes: m22-blue-[0-1],m22-green-3
    - switch: s0_0_1
      nodes: m22-blue-2
    - switch: s0_1
      children: s0_1_0
    - switch: s0_1_0
      nodes: m22-blue-3
    - switch: s0_2
      nodes: m22-blue-[4-6]
    - switch: s0_3
      nodes: m22-green-[0-2,4]
    - switch: s0_4
      nodes: m22-pink-[0-3]
    - switch: s1
      children: s1_[0-1]
    - switch: s1_0
      children: s1_0_[0-3]
    - switch: s1_0_0
      nodes: m22-bold-[0-2]
    - switch: s1_0_1
      nodes: m22-bold-3
    - switch: s1_0_2
      nodes: m22-bold-[4-6]
    - switch: s1_0_3
      nodes: m22-bold-[7-8]
    - switch: s1_1
      nodes: m22-slim-[0-2]
"""
    expected_parsed_tree = yaml.safe_load(expected_tree_yaml)[0]
    assert tree_topology == expected_parsed_tree

    # Assert topology/block section
    block_topology = parsed_yaml[1]
    assert block_topology['topology'] == 'topology/block'
    assert block_topology['cluster_default'] == False
    assert block_topology['block']['block_sizes'] == [NVLINK_VM_COUNT, NVLINK_VM_COUNT * BLOCK_SIZE]

    blocks_list = block_topology['block']['blocks']
    
    # Assert total count of blocks
    # 5 block groups: 'a', 'b', 'slurm-root', 'ns_bold', 'ns_slim'
    # 'a': 2 actual blocks, (32-2)=30 phantom blocks
    # 'b': 1 actual block, (32-1)=31 phantom blocks
    # 'slurm-root': 1 actual block, (32-1)=31 phantom blocks
    # 'ns_bold': 4 actual blocks, (32-4)=28 phantom blocks
    # 'ns_slim': 0 actual blocks, 0 phantom blocks (no blocks generated for 'ns_slim')
    # Total actual: 2+1+1+4+0 = 8
    # Total phantom: 30+31+31+28+0 = 120
    # Sum: 8 + 120 = 128.
    assert len(blocks_list) == 128
    # Assert actual blocks and their names/UUID format
    extracted_actual_blocks_data = []
    for block_entry in blocks_list:
        nodes = block_entry['nodes']
        if nodes != '': # This is an actual block
            block_name = block_entry['block']
            parts = block_name.rsplit('-', 1)
            assert len(parts) == 2, f"Block name '{block_name}' does not have the expected format"
            name_part, uuid_suffix = parts

            assert re.fullmatch(r'[0-9a-fA-F]{7}', uuid_suffix), \
                   f"UUID suffix '{uuid_suffix}' in block name '{block_name}' is not a 7-character hex string"

            extracted_actual_blocks_data.append({
                'name_part': name_part,
                'nodes': nodes,
            })
    # Expected name_part and nodes for actual blocks (based on detailed tracing and EXTRACTED ACTUAL BLOCKS DATA)
    expected_actual_blocks_data = [
        {'name_part': 'bold-0', 'nodes': 'm22-bold-[0-2]'},
        {'name_part': 'bold-1', 'nodes': 'm22-bold-3'},
        {'name_part': 'bold-2', 'nodes': 'm22-bold-[4-6]'},
        {'name_part': 'bold-3', 'nodes': 'm22-bold-[7-8]'},
        {'name_part': 'slurm-root', 'nodes': 'm22-blue-[4-6],m22-green-[0-2,4],m22-pink-[0-3],m22-slim-[0-2]'},
        {'name_part': 'a', 'nodes': 'm22-blue-[0-1],m22-green-3'},
        {'name_part': 'b', 'nodes': 'm22-blue-2'},
        {'name_part': 'a', 'nodes': 'm22-blue-3'}
    ]
    assert extracted_actual_blocks_data == unordered(expected_actual_blocks_data)

    # Assert phantom blocks
    phantom_counts = {
        'a': 0,
        'b': 0,
        'slurm-root': 0,
        'ns_bold': 0,
    }
    for block_entry in blocks_list:
        nodes = block_entry['nodes']
        if nodes == '': # This is a phantom block
            # Phantom block names are f"{block_group_name[:10]}-p{phantom}"
            parts = block_entry['block'].rsplit('-p', 1)
            prefix = parts[0] if len(parts) == 2 else block_entry['block']
            assert prefix in phantom_counts, f"Unexpected phantom block prefix: {prefix}"
            phantom_counts[prefix] += 1
    assert phantom_counts['a'] == (BLOCK_SIZE - 2) # Found 30 phantom blocks, so 2 actual blocks.
    assert phantom_counts['b'] == (BLOCK_SIZE - 1) # Found 31 phantom blocks, so 1 actual block.
    assert phantom_counts['slurm-root'] == (BLOCK_SIZE - 1) # Found 31 phantom blocks, so 1 actual block.
    assert phantom_counts['ns_bold'] == (BLOCK_SIZE - 4) # Found 28 phantom blocks, so 4 actual blocks.

    summary.dump(lkp)
    summary_got = json.loads(open(output_dir + "/cloud_topology.summary.json").read())

    assert summary_got == {
        "down_nodes": unordered(
            [f"m22-blue-{i}" for i in (4,5,6)] +
            [f"m22-green-{i}" for i in (0,1,2,4)] +
            [f"m22-pink-{i}" for i in range(4)]),
        "tpu_nodes": unordered(
            [f"m22-bold-{i}" for i in range(9)] +
            [f"m22-slim-{i}" for i in range(3)]),
        'physical_host': {
            'm22-blue-0': '/a/a/a',
            'm22-blue-1': '/a/a/b',
            'm22-blue-2': '/a/b/a',
            'm22-blue-3': '/b/a/a',
            'm22-green-3': '/a/a/c'},
    }


def test_gen_topology_conf_update():
    cfg = TstCfg(
        nodeset={
            "c": TstNodeset("green", node_count_static=2),
        },
        output_dir=tempfile.mkdtemp(),
    )
    lkp = util.Lookup(cfg)
    lkp.instances = lambda: { # type: ignore[assignment]
        # no instances
    }

    # initial generation - reconfigure
    upd, sum = conf.gen_topology_yaml(lkp)
    assert upd == True
    sum.dump(lkp)

    # add node: node_count_static 2 -> 3 - reconfigure
    lkp.cfg.nodeset["c"].node_count_static = 3
    upd, sum = conf.gen_topology_yaml(lkp)
    assert upd == True
    sum.dump(lkp)

    # remove node: node_count_static 3 -> 2  - no reconfigure
    lkp.cfg.nodeset["c"].node_count_static = 2
    upd, sum = conf.gen_topology_yaml(lkp)
    assert upd == False
    # don't dump

    # set empty physicalHost - no reconfigure
    lkp.instances = lambda: { # type: ignore[assignment]
        n.name: n for n in [tstInstance("m22-green-0", physical_host="")]}
    upd, sum = conf.gen_topology_yaml(lkp)
    assert upd == False
    # don't dump

    # set physicalHost - reconfigure
    lkp.instances = lambda: { # type: ignore[assignment]
        n.name: n for n in [tstInstance("m22-green-0", physical_host="/a/b/c")]}
    upd, sum = conf.gen_topology_yaml(lkp)
    assert upd == True
    sum.dump(lkp)

    # change physicalHost - reconfigure
    lkp.instances = lambda: { # type: ignore[assignment]
        n.name: n for n in [tstInstance("m22-green-0", physical_host="/a/b/z")]}
    upd, sum = conf.gen_topology_yaml(lkp)
    assert upd == True
    sum.dump(lkp)

    # shut down node - no reconfigure
    lkp.instances = lambda: {} # type: ignore[assignment]
    upd, sum = conf.gen_topology_yaml(lkp)
    assert upd == False
    # don't dump


@pytest.mark.parametrize(
    "paths,expected",
    [
        (["z/n-0", "z/n-1", "z/n-2", "z/n-3", "z/n-4", "z/n-10"], ['n-0', 'n-1', 'n-2', 'n-3', 'n-4', 'n-10']),
        (["y/n-0", "z/n-1", "x/n-2", "x/n-3", "y/n-4", "g/n-10"], ['n-0', 'n-4', 'n-1', 'n-2', 'n-3', 'n-10']),
    ])
def test_sort_nodes_order(paths: list[str], expected: list[str]) -> None:
    paths_expanded = [l.split("/") for l in paths]
    assert sort_nodes.order(paths_expanded) == expected

@mock.patch('util.Lookup.slurm_version', new_callable=mock.PropertyMock)
@mock.patch('uuid.uuid4')
def test_generate_topology_for_slurm_25_05(mock_uuid, mock_slurm_version):
    mock_uuid.side_effect = [
        mock.MagicMock(hex='00000' + '0' * 27),
        mock.MagicMock(hex='00001' + '0' * 27),
    ]
    mock_slurm_version.return_value = "25.05"
    output_dir = tempfile.mkdtemp()
    cfg = TstCfg(
        output_dir=output_dir,
        nodeset={"ns1": TstNodeset("ns1", node_count_static=2)},
    )
    lkp = util.Lookup(cfg)
    lkp.instances = lambda: { # type: ignore[assignment]
        "m22-ns1-0": tstInstance("m22-ns1-0", physical_host="/rack1/host1")
    }

    updated, summary = conf.gen_topology_yaml(lkp)

    assert updated is True
    assert summary.physical_host == {"m22-ns1-0": "/rack1/host1"}
    assert summary.down_nodes == {"m22-ns1-1"}

    yaml_file = Path(output_dir) / "cloud_topology.yaml"
    assert yaml_file.exists()
    yaml_content = yaml_file.read_text()

    parsed_yaml = yaml.safe_load(yaml_content)

    # Assert topology/tree section
    tree_topology = parsed_yaml[0]
    assert tree_topology['topology'] == 'topology/tree'
    assert tree_topology['cluster_default'] == True
    
    expected_tree_yaml = PRELUDE + """---

- topology: topology/tree
  cluster_default: true
  tree:
    switches:
    - switch: s0
      children: s0_[0-1]
    - switch: s0_0
      nodes: m22-ns1-1
    - switch: s0_1
      children: s0_1_0
    - switch: s0_1_0
      nodes: m22-ns1-0
"""
    expected_parsed_tree = yaml.safe_load(expected_tree_yaml)[0]
    assert tree_topology == expected_parsed_tree

    # Assert topology/block section
    block_topology = parsed_yaml[1]
    assert block_topology['topology'] == 'topology/block'
    assert block_topology['cluster_default'] == False
    assert block_topology['block']['block_sizes'] == [NVLINK_VM_COUNT, NVLINK_VM_COUNT * BLOCK_SIZE]

    blocks_list = block_topology['block']['blocks']
    
    # Assert total count of blocks
    assert len(blocks_list) == 64 # 1+31 for 'rack1', 1+31 for 'slurm-root'
    
    # Assert actual blocks
    extracted_actual_blocks_data = []
    for block_entry in blocks_list:
        block_name = block_entry['block']
        nodes = block_entry['nodes']
        if nodes != '': # This is an actual block
            parts = block_name.rsplit('-', 1)
            assert len(parts) == 2, f"Block name '{block_name}' does not have the expected format"
            name_prefix, uuid_suffix = parts

            assert re.fullmatch(r'[0-9a-fA-F]{7}', uuid_suffix), \
                   f"UUID suffix '{uuid_suffix}' in block name '{block_name}' is not a 7-character hex string"

            extracted_actual_blocks_data.append({
                'name_prefix': name_prefix,
                'nodes': nodes
            })

    expected_actual_blocks_data = [
        {'name_prefix': 'host1', 'nodes': 'm22-ns1-0'},
        {'name_prefix': 'slurm-root', 'nodes': 'm22-ns1-1'},
    ]
    
    assert extracted_actual_blocks_data == unordered(expected_actual_blocks_data)
    
    # Assert phantom blocks
    phantom_counts = { 'rack1': 0, 'slurm-root': 0 }
    for block_entry in blocks_list:
        block_name = block_entry['block']
        nodes = block_entry['nodes']
        if nodes == '':
            prefix = '-'.join(block_name.split('-')[:-1])
            if prefix in phantom_counts:
                phantom_counts[prefix] += 1
                
    assert phantom_counts['rack1'] == (BLOCK_SIZE - 1)
    assert phantom_counts['slurm-root'] == (BLOCK_SIZE - 1)


@mock.patch("conf_v2411.gen_topology_conf")
@mock.patch('util.Lookup.slurm_version', new_callable=mock.PropertyMock)
def test_generate_topology_for_slurm_24_11(mock_slurm_version, mock_gen_topo_conf):
    """Verify that old topology format is used for Slurm < 25.05"""
    mock_slurm_version.return_value = "24.11"
    output_dir = tempfile.mkdtemp()
    cfg = TstCfg(output_dir=output_dir)
    lkp = util.Lookup(cfg)

    conf_v2411.gen_topology_conf(lkp)

    mock_gen_topo_conf.assert_called_once_with(lkp)
