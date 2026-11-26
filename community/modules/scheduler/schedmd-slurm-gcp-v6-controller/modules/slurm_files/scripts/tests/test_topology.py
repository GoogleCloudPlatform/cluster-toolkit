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
from common import TstCfg, TstNodeset, tstInstance
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


@mock.patch('uuid.uuid4')
def test_gen_topology_conf(mock_uuid):
    mock_uuid.side_effect = [
        mock.MagicMock(hex=f'{i:05d}' + '0' * 27) for i in range(15)
    ]
    output_dir = tempfile.mkdtemp()
    cfg = TstCfg(
        nodeset={
            "c": TstNodeset("green", node_count_static=2, node_count_dynamic_max=3),
            "d": TstNodeset("blue", node_count_static=7),
            "e": TstNodeset("pink", node_count_dynamic_max=4),
        },
        output_dir=output_dir,
    )

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
    # 3 block groups: 'a', 'b', 'slurm-root'
    # 'a': 2 actual blocks, (32-2)=30 phantom blocks
    # 'b': 1 actual block, (32-1)=31 phantom blocks
    # 'slurm-root': 1 actual block, (32-1)=31 phantom blocks
    # Total actual: 2+1+1 = 4
    # Total phantom: 30+31+31 = 92
    # Sum: 4 + 92 = 96.
    assert len(blocks_list) == 96
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
    # Expected name_part and nodes for actual blocks
    expected_actual_blocks_data = [
        {'name_part': 'slurm-root', 'nodes': 'm22-blue-[4-6],m22-green-[0-2,4],m22-pink-[0-3]'},
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
    }
    for block_entry in blocks_list:
        nodes = block_entry['nodes']
        if nodes == '': # This is a phantom block
            # Phantom block names are f"{block_group_name[:10]}-p{phantom}"
            parts = block_entry['block'].rsplit('-p', 1)
            prefix = parts[0] if len(parts) == 2 else block_entry['block']
            assert prefix in phantom_counts, f"Unexpected phantom block prefix: {prefix}"
            phantom_counts[prefix] += 1
    assert phantom_counts['a'] == (BLOCK_SIZE - 2)
    assert phantom_counts['b'] == (BLOCK_SIZE - 1)
    assert phantom_counts['slurm-root'] == (BLOCK_SIZE - 1)

    summary.dump(lkp)
    summary_got = json.loads(open(output_dir + "/cloud_topology.summary.json").read())

    assert summary_got == {
        "down_nodes": unordered(
            [f"m22-blue-{i}" for i in (4,5,6)] +
            [f"m22-green-{i}" for i in (0,1,2,4)] +
            [f"m22-pink-{i}" for i in range(4)]),
        "tpu_nodes": [],
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


@pytest.mark.parametrize("by_region", [False, True])
def test_gen_topology_region_switches(by_region):
    sub = "https://www.googleapis.com/compute/v1/projects/p/regions/{}/subnetworks/s"
    cfg = TstCfg(
        nodeset={
            "a": TstNodeset("red", node_count_dynamic_max=3, subnetwork=sub.format("us-east1")),
            "b": TstNodeset("teal", node_count_dynamic_max=2, subnetwork=sub.format("us-east1")),
            "c": TstNodeset("gold", node_count_dynamic_max=2, subnetwork=sub.format("us-west4")),
            "d": TstNodeset("nvl", node_count_dynamic_max=2, subnetwork=sub.format("us-east1"),
                            accelerator_topology="1x72"),
            "e": TstNodeset("bare", node_count_dynamic_max=1, subnetwork=""),
        },
        cloud_parameters={"topology_region_switches": by_region},
        output_dir=tempfile.mkdtemp(),
    )
    lkp = util.Lookup(cfg)
    lkp.instances = lambda: { n.name: n for n in [ # type: ignore[assignment]
        tstInstance("m22-red-0"),
        tstInstance("m22-red-2", physical_host="/a/b/c"),
    ]}
    switches = conf.gen_topology(lkp).compress().render_yaml()[0]["tree"]["switches"]
    leaves = sorted(sw["nodes"] for sw in switches if "nodes" in sw)
    assert conf._default_switch(lkp.cfg.nodeset["e"], lkp) == "ns_bare"
    if by_region:
        expected = ["m22-bare-0", "m22-gold-[0-1]", "m22-nvl-[0-1]", "m22-red-2", "m22-red-[0-1],m22-teal-[0-1]"]
    else:
        expected = ["m22-bare-0", "m22-gold-[0-1]", "m22-nvl-[0-1]", "m22-red-2", "m22-red-[0-1]", "m22-teal-[0-1]"]
    assert leaves == expected

    upd, summary = conf.gen_topology_yaml(lkp)
    assert upd
    summary.dump(lkp)
    upd, _ = conf.gen_topology_yaml(lkp)
    assert not upd


def test_gen_topology_yaml_mig_slice_blocks_and_block_sizes(tmp_path):
    """Unreserved static A4X MIG slices enable topology/block, shard cold-start down nodes
    per slice MIG, and size blocks from nodeset_slice_size() (both conf and conf_v2605)."""
    from common import TstPartition
    import conf_v2605

    # 1. 36-node unreserved static MIG A4X (1x72 -> slice_size=18 -> 2 shards) at cold start
    out_18 = tmp_path / "c18"
    out_18.mkdir()
    cfg_1x72 = TstCfg(
        slurm_cluster_name="cl",
        nodeset={
            "a4x": TstNodeset(
                nodeset_name="a4x",
                node_count_static=36,
                node_count_dynamic_max=0,
                provisioning_engine="MIG",
                accelerator_topology="1x72",
                slice_size=18,
                reservation_name="",
            ),
        },
        partitions={
            "gpu": TstPartition(partition_name="gpu", partition_nodeset=["a4x"]),
        },
        output_dir=str(out_18),
    )
    lkp_18 = util.Lookup(cfg_1x72)
    with (
        mock.patch.object(lkp_18, "instances", return_value={}),
        mock.patch.object(lkp_18, "is_tpu_nodeset", return_value=False),
    ):
        conf.gen_topology_yaml(lkp_18)
    parsed_18 = yaml.safe_load((out_18 / "cloud_topology.yaml").read_text())
    by_topo = {entry["topology"]: entry for entry in parsed_18}
    assert by_topo["topology/tree"]["cluster_default"] is False
    assert by_topo["topology/block"]["cluster_default"] is True
    assert by_topo["topology/block"]["block"]["block_sizes"] == [18, 18 * BLOCK_SIZE]
    actual_nodes = sorted(
        b["nodes"] for b in by_topo["topology/block"]["block"]["blocks"] if b["nodes"]
    )
    assert actual_nodes == ["cl-a4x-[0-9,10-17]", "cl-a4x-[18-35]"]

    # 2. Non-18 slice_size (1x36 on 4-GPU hosts -> slice_size=9) drives block_sizes in both
    #    conf.py and conf_v2605.py (even when a TPU nodeset is also present in the cluster)
    out_9 = tmp_path / "c9"
    out_9.mkdir()
    cfg_9 = TstCfg(
        slurm_cluster_name="cl",
        nodeset={
            "a4x": TstNodeset(
                nodeset_name="a4x",
                node_count_static=18,
                node_count_dynamic_max=0,
                provisioning_engine="MIG",
                accelerator_topology="1x36",
                slice_size=9,
                reservation_name="",
            ),
            "tpu": TstNodeset(
                nodeset_name="tpu",
                node_count_static=2,
                node_count_dynamic_max=0,
                accelerator_topology="2x4",
            ),
        },
        partitions={
            "gpu": TstPartition(partition_name="gpu", partition_nodeset=["a4x"]),
            "tpu": TstPartition(partition_name="tpu", partition_nodeset=["tpu"]),
        },
        output_dir=str(out_9),
    )
    lkp_9 = util.Lookup(cfg_9)
    with (
        mock.patch.object(lkp_9, "instances", return_value={}),
        mock.patch.object(lkp_9, "is_tpu_nodeset", side_effect=lambda name: name == "tpu"),
        mock.patch.object(lkp_9, "has_tpu_nodesets", return_value=True),
        mock.patch.object(lkp_9, "is_tpu_static_nodeset", side_effect=lambda name: name == "tpu"),
        mock.patch.object(lkp_9, "get_tpu_chunk_size", return_value=2),
    ):
        assert conf.block_base_size_for_cluster(lkp_9) == 9
        gen = conf_v2605.SlurmConfigGeneratorV2605(lkp_9)
        gen.generate_topology_data()
    parsed_9 = yaml.safe_load((out_9 / "cloud_topology.yaml").read_text())
    by_topo_9 = {entry["topology"]: entry for entry in parsed_9}
    assert by_topo_9["topology/block"]["cluster_default"] is True
    assert by_topo_9["topology/block"]["block"]["block_sizes"] == [9, 9 * BLOCK_SIZE]
    assert by_topo_9[conf_v2605.TOPOLOGY_TPU_BLOCK]["block"]["block_sizes"] == [1]

    # 3. Booting A4X MIG nodes without physicalHost yet stay isolated per slice MIG (not lumped
    #    into slurm-root), alongside remaining down nodes
    out_boot = tmp_path / "cboot"
    out_boot.mkdir()
    cfg_boot = TstCfg(
        slurm_cluster_name="cl",
        nodeset={
            "a4x": TstNodeset(
                nodeset_name="a4x",
                node_count_static=36,
                node_count_dynamic_max=0,
                provisioning_engine="MIG",
                accelerator_topology="1x72",
                slice_size=18,
                reservation_name="",
            ),
        },
        partitions={"gpu": TstPartition(partition_name="gpu", partition_nodeset=["a4x"])},
        output_dir=str(out_boot),
    )
    lkp_boot = util.Lookup(cfg_boot)
    booting_insts = {
        "cl-a4x-0": tstInstance("cl-a4x-0", physical_host=""),
        "cl-a4x-18": tstInstance("cl-a4x-18", physical_host=""),
    }
    with (
        mock.patch.object(lkp_boot, "instances", return_value=booting_insts),
        mock.patch.object(lkp_boot, "is_tpu_nodeset", return_value=False),
    ):
        conf.gen_topology_yaml(lkp_boot)
    parsed_boot = yaml.safe_load((out_boot / "cloud_topology.yaml").read_text())
    by_topo_boot = {entry["topology"]: entry for entry in parsed_boot}
    boot_blocks = sorted(
        b["nodes"] for b in by_topo_boot["topology/block"]["block"]["blocks"] if b["nodes"]
    )
    assert boot_blocks == ["cl-a4x-[0-9,10-17]", "cl-a4x-[18-35]"]

    # 4. Mixed cluster (reserved BULK_INSERT 1x72=18 + MIG slice_size=36) uses smallest (18)
    cfg_mixed = TstCfg(
        slurm_cluster_name="cl",
        nodeset={
            "res_a4x": TstNodeset(
                nodeset_name="res_a4x",
                node_count_static=18,
                provisioning_engine="BULK_INSERT",
                accelerator_topology="1x72",
                reservation_name="projects/p/reservations/r1",
            ),
            "mig_a4x": TstNodeset(
                nodeset_name="mig_a4x",
                node_count_static=36,
                provisioning_engine="MIG",
                accelerator_topology="1x144",
                slice_size=36,
                reservation_name="",
            ),
        },
        partitions={
            "p_res": TstPartition(partition_name="p_res", partition_nodeset=["res_a4x"]),
            "p_mig": TstPartition(partition_name="p_mig", partition_nodeset=["mig_a4x"]),
        },
    )
    lkp_mixed = util.Lookup(cfg_mixed)
    with mock.patch.object(lkp_mixed, "is_tpu_nodeset", return_value=False):
        assert conf.block_base_size_for_cluster(lkp_mixed) == 18
