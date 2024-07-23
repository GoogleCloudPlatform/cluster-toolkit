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

import mock
from common import TstCfg, TstNodeset, TstTPU, make_to_hostnames_mock

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


@mock.patch("util.TPU")
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

    def tpu_se(ns: TstNodeset) -> TstTPU:
        if ns.nodeset_name == "bold":
            return TstTPU(vmcount=3)
        if ns.nodeset_name == "slim":
            return TstTPU(vmcount=1)
        raise AssertionError(f"unexpected TPU name: '{ns.nodeset_name}'")

    tpu_mock.side_effect = tpu_se

    lkp = util.Lookup(cfg)
    uncompressed = conf.gen_topology(lkp)
    want_uncompressed = [
        "SwitchName=nodeset-root Switches=blue,green,pink",
        "SwitchName=blue Nodes=m22-blue-[0-6]",
        "SwitchName=green Nodes=m22-green-[0-4]",
        "SwitchName=pink Nodes=m22-pink-[0-3]",
        "SwitchName=nodeset_tpu-root Switches=bold,slim",
        "SwitchName=bold Switches=bold-[0-3]",
        "SwitchName=bold-0 Nodes=m22-bold-[0-2]",
        "SwitchName=bold-1 Nodes=m22-bold-3",
        "SwitchName=bold-2 Nodes=m22-bold-[4-6]",
        "SwitchName=bold-3 Nodes=m22-bold-[7-8]",
        "SwitchName=slim Nodes=m22-slim-[0-2]"]
    assert list(uncompressed.render_conf_lines()) == want_uncompressed
        
    compressed = uncompressed.compress()
    want_compressed = [
        "SwitchName=s0 Switches=s0_[0-2]",
        "SwitchName=s0_0 Nodes=m22-blue-[0-6]",
        "SwitchName=s0_1 Nodes=m22-green-[0-4]",
        "SwitchName=s0_2 Nodes=m22-pink-[0-3]",
        "SwitchName=s1 Switches=s1_[0-1]",
        "SwitchName=s1_0 Switches=s1_0_[0-3]",
        "SwitchName=s1_0_0 Nodes=m22-bold-[0-2]",
        "SwitchName=s1_0_1 Nodes=m22-bold-3",
        "SwitchName=s1_0_2 Nodes=m22-bold-[4-6]",
        "SwitchName=s1_0_3 Nodes=m22-bold-[7-8]",
        "SwitchName=s1_1 Nodes=m22-slim-[0-2]"]
    assert list(compressed.render_conf_lines()) == want_compressed

    conf.gen_topology_conf(util.Lookup(cfg))
    want_written = PRELUDE + "\n".join(want_compressed) + "\n\n"
    assert open(cfg.output_dir + "/cloud_topology.conf").read() == want_written
