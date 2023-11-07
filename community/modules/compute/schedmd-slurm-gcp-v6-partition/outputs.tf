# Copyright 2023 Google LLC
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

output "partitions" {
  description = "Details of a slurm partition"

  value = [local.partition]

  precondition {
    condition     = (length(local.use_placement) == 0) || var.exclusive
    error_message = "If any nodeset `enable_placement`, `var.exclusive` must be set true"
  }

  precondition {
    condition     = (length(local.use_placement) == 0) || contains(["NO", "Exclusive"], lookup(var.partition_conf, "Oversubscribe", "NO"))
    error_message = "If any nodeset `enable_placement`, var.partition_conf[\"Oversubscribe\"] should be either undefined, \"NO\", or \"Exclusive\"."
  }

  precondition {
    condition     = (length(local.use_placement) == 0) || (lookup(var.partition_conf, "SuspendTime", null) == null)
    error_message = "If any nodeset `enable_placement`, var.partition_conf[\"SuspendTime\"] should be undefined."
  }
}

output "nodeset" {
  description = "Details of a nodesets in this partition"

  value = var.nodeset
}

output "nodeset_tpu" {
  description = "Details of a nodesets tpu in this partition"

  value = var.nodeset_tpu
}
