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
    condition     = (length(local.non_static_ns_with_placement) == 0) || var.exclusive
    error_message = "If any non-static nodesets has `enable_placement`, `var.exclusive` must be set true"
  }

  precondition {
    condition     = (length(local.use_static) == 0) || !var.exclusive
    error_message = <<-EOD
    Can't use static nodes within partition with `var.exclusive` set to `true`.
    NOTE: Partition's `var.exclusive` is set to `true` by default. Set it to `false` explicitly to use static nodes.
    EOD
  }

  precondition {
    # Can not mix TPU with other non-TPU nodesets due to SlurmGCP specific limitations;
    # Can not mix dynamic with non-dynamic nodesets due to Slurms inability to 
    # turn off "power management" at nodeset level (can only do it at partition or node level).
    condition     = sum([for b in [local.has_node, local.has_dyn, local.has_tpu] : b ? 1 : 0]) == 1
    error_message = "Partition must contain exactly one type of nodeset."
  }

  precondition {
    condition     = !local.uses_job_duration || var.exclusive
    error_message = "`use_job_duration` can only be used in exclusive partitions"
  }
}

output "nodeset" {
  description = "Details of a nodesets in this partition"

  value = var.nodeset
}

output "nodeset_tpu" {
  description = "Details of a TPU nodesets in this partition"

  value = var.nodeset_tpu
}


output "nodeset_dyn" {
  description = "Details of a dynamic nodesets in this partition"

  value = var.nodeset_dyn
}
