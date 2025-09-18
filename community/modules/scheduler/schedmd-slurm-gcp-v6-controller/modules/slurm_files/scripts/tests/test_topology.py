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

import pytest
import json
import mock
from pytest_unordered import unordered
from common import TstCfg, TstNodeset, TstTPU, tstInstance
import sort_nodes

import util
import conf
import tempfile

PRELUDE = """
# Warning:
# This file is managed by a script. Manual modifications will be overwritten.
---
"""

def test_gen_topology_yaml_empty():
    out_dir = tempfile.mkdtemp()
    cfg = TstCfg(output_dir=out_dir)
    conf.gen_topology_yaml(util.Lookup(cfg))
    assert open(out_dir + "/cloud_topology.yaml").read() ==  PRELUDE + """
- cluster_default: true
  topology: topo
  tree:
    switches: []
"""


def sw(name: str, switches: str|None=None, nodes: str|None=None) -> dict[str, str]:
    assert (switches is None) != (nodes is None)
    d = {"switch": name}
    if switches is not None:
        d["children"] = switches
    if nodes is not None:
        d["nodes"] = nodes
    return d



@mock.patch("tpu.TPU.make")
def test_gen_topology_yaml(tpu_mock):
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

    uncompressed = conf.gen_topology(lkp)
    want_uncompressed = [ 
        #NOTE: the switch names are not unique, it's not valid content for topology.conf
        # The uniquefication and compression of names are done in the compress() method
        sw("slurm-root", switches="a,b,ns_blue,ns_green,ns_pink"),
        # "physical" topology
        sw("a", switches="a,b"),
        sw("a", nodes="m22-blue-[0-1],m22-green-3"),
        sw("b", nodes="m22-blue-2"),
        sw("b", switches="a"),
        sw("a", nodes="m22-blue-3"),
        # topology "by nodeset"
        sw("ns_blue", nodes="m22-blue-[4-6]"),
        sw("ns_green", nodes="m22-green-[0-2,4]"),
        sw("ns_pink", nodes="m22-pink-[0-3]"),
        # TPU topology
        sw("tpu-root", switches="ns_bold,ns_slim"),
        sw("ns_bold", switches="bold-[0-3]"),
        sw("bold-0", nodes="m22-bold-[0-2]"),
        sw("bold-1", nodes="m22-bold-3"),
        sw("bold-2", nodes="m22-bold-[4-6]"),
        sw("bold-3", nodes="m22-bold-[7-8]"),
        sw("ns_slim", nodes="m22-slim-[0-2]")]
    assert uncompressed.render_yaml()[0]["tree"]["switches"] == unordered(want_uncompressed)
        
    compressed = uncompressed.compress()
    want_compressed = [
        sw("s0", switches="s0_[0-4]"), # root
        # "physical" topology
        sw("s0_0", switches="s0_0_[0-1]"), # /a
        sw("s0_0_0", nodes="m22-blue-[0-1],m22-green-3"), # /a/a
        sw("s0_0_1", nodes="m22-blue-2"),  # /a/b
        sw("s0_1", switches="s0_1_0"),  # /b
        sw("s0_1_0", nodes="m22-blue-3"),  # /b/a
        # topology "by nodeset"
        sw("s0_2", nodes="m22-blue-[4-6]"),
        sw("s0_3", nodes="m22-green-[0-2,4]"),
        sw("s0_4", nodes="m22-pink-[0-3]"),
        # TPU topology
        sw("s1", switches="s1_[0-1]"),
        sw("s1_0", switches="s1_0_[0-3]"),
        sw("s1_0_0", nodes="m22-bold-[0-2]"),
        sw("s1_0_1", nodes="m22-bold-3"),
        sw("s1_0_2", nodes="m22-bold-[4-6]"),
        sw("s1_0_3", nodes="m22-bold-[7-8]"),
        sw("s1_1", nodes="m22-slim-[0-2]")]
    assert compressed.render_yaml()[0]["tree"]["switches"] == unordered(want_compressed)

    upd, summary = conf.gen_topology_yaml(lkp)
    assert upd == True
    
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



def test_gen_topology_yaml_update():
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
