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

from typing import Any, List, Dict

from dataclasses import dataclass, field

@dataclass(frozen=True)
class Partition:
  name: str
  enable_job_exclusive: bool = False
  conf: Dict[str, Any] = field(default_factory=dict)

  nodesets: List[str] = field(default_factory=list)
  nodesets_dyn: List[str] = field(default_factory=list)
  nodesets_tpu: List[str] = field(default_factory=list)

  @property
  def is_tpu(self) -> bool:
    return len(self.nodesets_tpu) > 0
  
  @property
  def any_dynamic(self) -> bool:
    return len(self.nodesets_dyn) > 0

  @classmethod
  def from_json(cls, jo: dict) -> "Partition":
    return cls(
      name=jo["partition_name"],
      enable_job_exclusive=jo["enable_job_exclusive"],
      conf=jo.get("partition_conf", {}),

      nodesets=jo.get("nodesets", []),
      nodesets_dyn=jo.get("nodesets_dyn", []),
      nodesets_tpu=jo.get("nodesets_tpu", []),
    )
