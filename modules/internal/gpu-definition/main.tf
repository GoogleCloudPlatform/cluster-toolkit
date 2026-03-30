/**
 * Copyright 2026 Google LLC
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

locals {
  # example state; terraform will ignore diffs if last element of URL matches
  # guest_accelerator = [
  #   {
  #     count = 1
  #     type  = "https://www.googleapis.com/compute/beta/projects/PROJECT/zones/ZONE/acceleratorTypes/nvidia-tesla-a100"
  #   },
  # ]
  accelerators_json    = jsondecode(file("${path.module}/../../../pkg/config/accelerators.json"))
  accelerator_machines = try(
    jsondecode(var.machine_configs).gpus,
    var.machine_configs.gpus,
    local.accelerators_json.gpus,
    {}
  )

  generated_guest_accelerator = try([local.accelerator_machines[var.machine_type]], [])

  # Select in priority order:
  # (1) var.guest_accelerator if not empty
  # (2) local.generated_guest_accelerator if not empty
  # (3) default to empty list if both are empty
  guest_accelerator = try(coalescelist(var.guest_accelerator, local.generated_guest_accelerator), [])
}

output "guest_accelerator" {
  description = "Sanitized list of the type and count of accelerator cards attached to the instance."
  value       = local.guest_accelerator
}

output "machine_type_guest_accelerator" {
  description = "List of the type and count of accelerator cards attached to the specified machine type."
  value       = local.generated_guest_accelerator
}

terraform {
  required_version = "= 1.12.2"
}
