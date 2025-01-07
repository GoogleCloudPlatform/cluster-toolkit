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

output "nodeset" {
  description = "Details of the nodeset. Typically used as input to `schedmd-slurm-gcp-v6-partition`."
  value       = local.nodeset

  precondition {
    condition = !contains([
      "c3-:pd-standard",
      "h3-:pd-standard",
      "h3-:pd-ssd",
    ], "${substr(var.machine_type, 0, 3)}:${var.disk_type}")
    error_message = "A disk_type=${var.disk_type} cannot be used with machine_type=${var.machine_type}."
  }

  precondition {
    condition     = var.reservation_name == "" || !var.enable_placement
    error_message = <<-EOD
      If a reservation is specified, `var.enable_placement` must be `false`.
      If the specified reservation has a placement policy then it will be used automatically.
    EOD
  }

  precondition {
    condition     = var.reservation_name == "" || length(var.zones) == 0
    error_message = <<-EOD
      If a reservation is specified, `var.zones` should be empty.
    EOD
  }

  precondition {
    condition     = !var.enable_placement || var.node_count_static == 0 || var.node_count_dynamic_max == 0
    error_message = "Cannot use placement with static and auto-scaling nodes in the same node set."
  }

  precondition {
    condition     = var.placement_max_distance == null || var.enable_placement
    error_message = "placement_max_distance requires enable_placement to be set to true."
  }

  precondition {
    condition     = !(startswith(var.machine_type, "a3-") && var.placement_max_distance == 1)
    error_message = "A3 machines do not support a placement_max_distance of 1."
  }

  precondition {
    condition     = var.reservation_name == "" || !var.dws_flex.enabled
    error_message = "Cannot use reservations with DWS Flex."
  }

  precondition {
    condition     = !var.enable_placement || !var.dws_flex.enabled
    error_message = "Cannot use DWS Flex with `enable_placement`."
  }

  precondition {
    condition     = var.reservation_name == "" || var.future_reservation == ""
    error_message = "Cannot use reservations and future reservations in the same nodeset"
  }

  precondition {
    condition     = !var.enable_placement || var.future_reservation == ""
    error_message = "Cannot use `enable_placement` with future reservations."
  }

  precondition {
    condition     = var.future_reservation == "" || length(var.zones) == 0
    error_message = <<-EOD
      If a future reservation is specified, `var.zones` should be empty.
    EOD
  }

  precondition {
    condition     = var.future_reservation == "" || local.fr_zone == var.zone
    error_message = <<-EOD
      The zone of the deployment must match that of the future reservation
    EOD
  }

  precondition {
    condition     = var.node_count_dynamic_max > 0 || var.node_count_static > 0
    error_message = <<-EOD
      This nodeset contains zero nodes, there should be at least one static or dynamic node
    EOD
  }
}
