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
from common import TstCfg, TstNodeset, TstTPU, TstInstance
import sort_nodes

import util
import conf
import tempfile

PRELUDE = """
# Warning:
# This file is managed by a script. Manual modifications will be overwritten.

"""

def test_gen_topology_conf_empty():
    cfg = TstCfg(output_dir=tempfile.mkdtemp())
    conf.gen_topology_conf(util.Lookup(cfg))
    assert open(cfg.output_dir + "/cloud_topology.conf").read() == PRELUDE + "\n"


@mock.patch("tpu.TPU.make")
def test_gen_topology_conf(tpu_mock):
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
        output_dir=tempfile.mkdtemp(),
    )

    def tpu_se(ns: str, lkp) -> TstTPU:
        if ns == "bold":
            return TstTPU(vmcount=3)
        if ns == "slim":
            return TstTPU(vmcount=1)
        raise AssertionError(f"unexpected TPU name: '{ns}'")

    tpu_mock.side_effect = tpu_se

    lkp = util.Lookup(cfg)
    lkp.instances = lambda: { n.name: n for n in [
        # nodeset blue
        TstInstance("m22-blue-0"),  # no physicalHost
        TstInstance("m22-blue-0", physicalHost="/a/a/a"),
        TstInstance("m22-blue-1", physicalHost="/a/a/b"),
        TstInstance("m22-blue-2", physicalHost="/a/b/a"),
        TstInstance("m22-blue-3", physicalHost="/b/a/a"),
        # nodeset green
        TstInstance("m22-green-3", physicalHost="/a/a/c"),
    ]}

    uncompressed = conf.gen_topology(lkp)
    want_uncompressed = [ 
        #NOTE: the switch names are not unique, it's not valid content for topology.conf
        # The uniquefication and compression of names are done in the compress() method
        "SwitchName=slurm-root Switches=a,b,ns_blue,ns_green,ns_pink",
        # "physical" topology
        'SwitchName=a Switches=a,b',
        'SwitchName=a Nodes=m22-blue-[0-1],m22-green-3',
        'SwitchName=b Nodes=m22-blue-2',
        'SwitchName=b Switches=a',
        'SwitchName=a Nodes=m22-blue-3',
        # topology "by nodeset"
        "SwitchName=ns_blue Nodes=m22-blue-[4-6]",
        "SwitchName=ns_green Nodes=m22-green-[0-2,4]",
        "SwitchName=ns_pink Nodes=m22-pink-[0-3]",
        # TPU topology
        "SwitchName=tpu-root Switches=ns_bold,ns_slim",
        "SwitchName=ns_bold Switches=bold-[0-3]",
        "SwitchName=bold-0 Nodes=m22-bold-[0-2]",
        "SwitchName=bold-1 Nodes=m22-bold-3",
        "SwitchName=bold-2 Nodes=m22-bold-[4-6]",
        "SwitchName=bold-3 Nodes=m22-bold-[7-8]",
        "SwitchName=ns_slim Nodes=m22-slim-[0-2]"]
    assert list(uncompressed.render_conf_lines()) == want_uncompressed
        
    compressed = uncompressed.compress()
    want_compressed = [
        "SwitchName=s0 Switches=s0_[0-4]", # root
        # "physical" topology
        'SwitchName=s0_0 Switches=s0_0_[0-1]', # /a
        'SwitchName=s0_0_0 Nodes=m22-blue-[0-1],m22-green-3', # /a/a
        'SwitchName=s0_0_1 Nodes=m22-blue-2',  # /a/b
        'SwitchName=s0_1 Switches=s0_1_0',  # /b
        'SwitchName=s0_1_0 Nodes=m22-blue-3',  # /b/a
        # topology "by nodeset"
        "SwitchName=s0_2 Nodes=m22-blue-[4-6]",
        "SwitchName=s0_3 Nodes=m22-green-[0-2,4]",
        "SwitchName=s0_4 Nodes=m22-pink-[0-3]",
        # TPU topology
        "SwitchName=s1 Switches=s1_[0-1]",
        "SwitchName=s1_0 Switches=s1_0_[0-3]",
        "SwitchName=s1_0_0 Nodes=m22-bold-[0-2]",
        "SwitchName=s1_0_1 Nodes=m22-bold-3",
        "SwitchName=s1_0_2 Nodes=m22-bold-[4-6]",
        "SwitchName=s1_0_3 Nodes=m22-bold-[7-8]",
        "SwitchName=s1_1 Nodes=m22-slim-[0-2]"]
    assert list(compressed.render_conf_lines()) == want_compressed

    upd, summary = conf.gen_topology_conf(lkp)
    assert upd == True
    want_written = PRELUDE + "\n".join(want_compressed) + "\n\n"
    assert open(cfg.output_dir + "/cloud_topology.conf").read() == want_written

    summary.dump(lkp)
    summary_got = json.loads(open(cfg.output_dir + "/cloud_topology.summary.json").read())
    
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
    lkp.instances = lambda: {} # no instances

    # initial generation - reconfigure
    upd, sum = conf.gen_topology_conf(lkp)
    assert upd == True
    sum.dump(lkp)

    # add node: node_count_static 2 -> 3 - reconfigure
    lkp.cfg.nodeset["c"].node_count_static = 3
    upd, sum = conf.gen_topology_conf(lkp)
    assert upd == True
    sum.dump(lkp)

    # remove node: node_count_static 3 -> 2  - no reconfigure
    lkp.cfg.nodeset["c"].node_count_static = 2
    upd, sum = conf.gen_topology_conf(lkp)
    assert upd == False
    # don't dump

    # set empty physicalHost - no reconfigure
    lkp.instances = lambda: { n.name: n for n in [TstInstance("m22-green-0", physicalHost="")]}
    upd, sum = conf.gen_topology_conf(lkp)
    assert upd == False
    # don't dump

    # set physicalHost - reconfigure
    lkp.instances = lambda: { n.name: n for n in [TstInstance("m22-green-0", physicalHost="/a/b/c")]}
    upd, sum = conf.gen_topology_conf(lkp)
    assert upd == True
    sum.dump(lkp)

    # change physicalHost - reconfigure
    lkp.instances = lambda: { n.name: n for n in [TstInstance("m22-green-0", physicalHost="/a/b/z")]}
    upd, sum = conf.gen_topology_conf(lkp)
    assert upd == True
    sum.dump(lkp)

    # shut down node - no reconfigure
    lkp.instances = lambda: {}
    upd, sum = conf.gen_topology_conf(lkp)
    assert upd == False
    # don't dump


@pytest.mark.parametrize(
    "paths,expected",
    [
        (["z/n-0", "z/n-1", "z/n-2", "z/n-3", "z/n-4", "z/n-10"], ['n-0', 'n-1', 'n-2', 'n-3', 'n-4', 'n-10']),
        (["y/n-0", "z/n-1", "x/n-2", "x/n-3", "y/n-4", "g/n-10"], ['n-0', 'n-4', 'n-1', 'n-2', 'n-3', 'n-10']),
    ])
def test_sort_nodes_order(paths: list[list[str]], expected: list[str]) -> None:
    paths = [l.split("/") for l in paths]
    assert sort_nodes.order(paths) == expected
