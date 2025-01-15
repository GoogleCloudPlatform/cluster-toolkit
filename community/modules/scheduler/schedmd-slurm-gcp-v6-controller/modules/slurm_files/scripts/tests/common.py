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

from typing import Optional, Any
import sys
from dataclasses import dataclass, field

SCRIPTS_DIR = "community/modules/scheduler/schedmd-slurm-gcp-v6-controller/modules/slurm_files/scripts"
if SCRIPTS_DIR not in sys.path:
    sys.path.append(SCRIPTS_DIR)  # TODO: make this more robust

import util

# TODO: use "real" classes once they are defined (instead of NSDict)

@dataclass
class Placeholder:
    pass

@dataclass
class TstNodeset:
    nodeset_name: str = "cantor"
    node_count_static: int = 0
    node_count_dynamic_max: int = 0
    node_conf: dict[str, Any] = field(default_factory=dict)
    instance_template: Optional[str] = None
    reservation_name: Optional[str] = ""
    zone_policy_allow: Optional[list[str]] = field(default_factory=list)
    enable_placement: bool = True
    placement_max_distance: Optional[int] = None

@dataclass
class TstPartition:
    partition_name: str = "euler"
    partition_nodeset: list[str] = field(default_factory=list)
    partition_nodeset_tpu: list[str] = field(default_factory=list)
    enable_job_exclusive: bool = False

@dataclass
class TstCfg:
    slurm_cluster_name: str = "m22"
    cloud_parameters: dict[str, Any] = field(default_factory=dict)

    partitions: dict[str, Placeholder] = field(default_factory=dict)
    nodeset: dict[str, TstNodeset] = field(default_factory=dict)
    nodeset_tpu: dict[str, TstNodeset] = field(default_factory=dict)
    nodeset_dyn: dict[str, TstNodeset] = field(default_factory=dict)
    
    install_dir: Optional[str] = None
    output_dir: Optional[str] = None

    prolog_scripts: Optional[list[Placeholder]] = field(default_factory=list)
    epilog_scripts: Optional[list[Placeholder]] = field(default_factory=list)
    

@dataclass
class TstTPU:  # to prevent client initialization durint "TPU.__init__"
    vmcount: int

@dataclass
class TstMachineConf:
    cpus: int
    memory: int
    sockets: int
    sockets_per_board: int
    cores_per_socket: int
    boards: int
    threads_per_core: int


@dataclass
class TstTemplateInfo:
    gpu: Optional[util.AcceleratorInfo]

@dataclass
class TstInstance:
    name: str
    region: str = "gondor"
    zone: str = "anorien"
    placementPolicyId: Optional[str] = None
    physicalHost: Optional[str] = None

    @property
    def resourceStatus(self):
        return {"physicalHost": self.physicalHost}

def make_to_hostnames_mock(tbl: Optional[dict[str, list[str]]]):
    tbl = tbl or {}

    def se(k: str) -> list[str]:
        if k not in tbl:
            raise AssertionError(f"to_hostnames mock: unexpected nodelist: '{k}'")
        return tbl[k]

    return se
