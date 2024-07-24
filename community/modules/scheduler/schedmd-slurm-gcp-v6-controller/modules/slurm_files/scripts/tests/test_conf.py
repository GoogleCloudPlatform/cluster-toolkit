# Copyright 2024 Google LLC
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
from mock import Mock
from common import TstNodeset, TstCfg, TstMachineConf, TstTemplateInfo

import conf
import util


def test_nodeset_tpu_lines():
    nodeset = TstNodeset(
        "turbo",
        node_count_static=2,
        node_count_dynamic_max=3,
        node_conf={"red": "velvet"},
    )
    assert conf.nodeset_tpu_lines(nodeset, util.Lookup(TstCfg())) == "\n".join(
        [
            "NodeName=m22-turbo-[0-4] State=CLOUD red=velvet",
            "NodeSet=turbo Nodes=m22-turbo-[0-4]",
        ]
    )


def test_nodeset_lines():
    nodeset = TstNodeset(
        "turbo",
        node_count_static=2,
        node_count_dynamic_max=3,
        node_conf={"red": "velvet", "CPUs": 55},
    )
    lkp = util.Lookup(TstCfg())
    lkp.template_info = Mock(return_value=TstTemplateInfo(gpu_count=33))
    mc = TstMachineConf(
        cpus=5,
        memory=6,
        sockets=7,
        sockets_per_board=8,
        boards=9,
        threads_per_core=10,
        cores_per_socket=11,
    )
    lkp.template_machine_conf = Mock(return_value=mc)
    assert conf.nodeset_lines(nodeset, lkp) == "\n".join(
        [
            "NodeName=m22-turbo-[0-4] State=CLOUD RealMemory=6 Boards=9 SocketsPerBoard=8 CoresPerSocket=11 ThreadsPerCore=10 CPUs=55 Gres=gpu:33 red=velvet",
            "NodeSet=turbo Nodes=m22-turbo-[0-4]",
        ]
    )


@pytest.mark.parametrize(
    "value,want",
    [
        ({"a": 1}, "a=1"),
        ({"a": "two"}, "a=two"),
        ({"a": [3, 4]}, "a=3,4"),
        ({"a": ["five", "six"]}, "a=five,six"),
        ({"a": None}, ""),
        ({"a": ["seven", None, 8]}, "a=seven,8"),
        ({"a": 1, "b": "two"}, "a=1 b=two"),
        ({"a": 1, "b": None, "c": "three"}, "a=1 c=three"),
        ({"a": 0, "b": None, "c": 0.0, "e": ""}, "a=0 c=0.0"),
        ({"a": [0, 0.0, None, "X", "", "Y"]}, "a=0,0.0,X,,Y"),
    ])
def test_dict_to_conf(value: dict, want: str):
    assert conf.dict_to_conf(value) == want
