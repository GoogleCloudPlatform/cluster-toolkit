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

output "nodeset_tpu" {
  description = "Details of the nodeset tpu. Typically used as input to `schedmd-slurm-gcp-v6-partition`."
  value       = local.nodeset_tpu

  precondition {
    condition     = (var.node_type == "") != (var.accelerator_config == { topology : "", version : "" })
    error_message = "Either a node_type or an accelerator_config must be provided."
  }

  precondition {
    condition     = ((local.tpu_core_count / 8) <= var.node_count_dynamic_max) || ((local.tpu_core_count / 8) <= var.node_count_static)
    error_message = <<-EOD
      When using TPUs there should be at least one node per every 8 cores. 
      Currently there are ${local.tpu_core_count} cores but only ${var.node_count_static} static nodes and ${var.node_count_dynamic_max} dynamic nodes.
    EOD
  }

  precondition {
    condition     = (var.node_count_dynamic_max % (local.tpu_core_count / 8) == 0) && (var.node_count_static % (local.tpu_core_count / 8) == 0)
    error_message = <<-EOD
      The number of worker nodes should be a multiple of ${local.tpu_core_count / 8}.
      This is to ensure each node has a TPU machine for job scheduling.
    EOD
  }
}
