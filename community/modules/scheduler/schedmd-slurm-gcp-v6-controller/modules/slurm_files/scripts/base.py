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

from typing import Optional, List

import addict
from dataclasses import dataclass
from datetime import datetime


def parse_gcp_timestamp(s: str) -> datetime:
  """
  Parse timestamp strings returned by GCP API into datetime.
  Works with both Zulu and non-Zulu timestamps.
  """
  # Requires Python >= 3.7
  # TODO: Remove this "hack" of trimming the Z from timestamps once we move to Python 3.11 
  # (context: https://discuss.python.org/t/parse-z-timezone-suffix-in-datetime/2220/30)
  return datetime.fromisoformat(s.replace('Z', '+00:00'))


@dataclass(frozen=True)
class Instance:
  name: str
  status: str
  creation_timestamp: datetime

  # TODO: use proper InstanceResourceStatus class
  resource_status: addict.Dict
  # TODO: use proper InstanceScheduling class
  scheduling: addict.Dict
  # TODO: use proper UpcomingMaintenance class
  upcoming_maintenance: Optional[addict.Dict] = None

  @classmethod
  def from_json(cls, jo: dict) -> "Instance":
    return cls(
      name=jo["name"],
      status=jo["status"],
      creation_timestamp=parse_gcp_timestamp(jo["creationTimestamp"]),
      resource_status=addict.Dict(jo["resourceStatus"]),
      scheduling=addict.Dict(jo["scheduling"]),
      upcoming_maintenance=addict.Dict(jo["upcomingMaintenance"]) if "upcomingMaintenance" in jo else None
    )
  

@dataclass(frozen=True)
class ReservationDetails:
    project: str
    zone: str
    name: str
    policies: List[str] # names (not URLs) of resource policies
    bulk_insert_name: str # name in format suitable for bulk insert (currently identical to user supplied name in long format)
    deployment_type: Optional[str]

    @property
    def dense(self) -> bool:
        return self.deployment_type == "DENSE"

@dataclass(frozen=True)
class FutureReservation:
    project: str
    zone: str
    name: str
    specific: bool
    start_time: datetime
    end_time: datetime
    active_reservation: Optional[ReservationDetails]
