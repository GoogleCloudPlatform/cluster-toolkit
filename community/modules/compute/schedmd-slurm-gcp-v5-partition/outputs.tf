/**
 * Copyright 2022 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

output "partition" {
  description = "Details of a slurm partition"
  value = {
    compute_list = module.slurm_partition.compute_list
    partition    = module.slurm_partition.partition
  }
  precondition {
    condition     = var.enable_placement == false || (var.exclusive == var.enable_placement)
    error_message = "If var.enable_placement is true, var.exclusive must be as well."
  }
  precondition {
    condition     = var.enable_placement == false || contains(["NO", "Exclusive"], lookup(var.partition_conf, "Oversubscribe", "NO"))
    error_message = "If var.enable_placement is true, var.partition_conf[\"Oversubscribe\"] should be either undefined, \"NO\", or \"Exclusive\"."
  }
  precondition {
    condition     = var.enable_placement == false || (lookup(var.partition_conf, "SuspendTime", null) == null)
    error_message = "If var.enable_placement is true, var.partition_conf[\"SuspendTime\"] should be undefined."
  }
  precondition {
    condition     = var.enable_placement == false || alltrue([for v in var.node_groups : v.reservation_name == ""])
    error_message = <<-EOT
      If `reservation_name` is specified in at least one node group, `var.enable_placement` must be false.
      If you want to use placement policies with reservations, the placement policy must be attached to the reservation.
      EOT
  }
}
