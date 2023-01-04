/**
 * Copyright 2023 Google LLC
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
  # if a machine type is a2-*-?g it will automatically fill in the guest_accelerator structure.
  is_a2_vm = length(regexall("a2-[a-z]+-\\d+g", var.machine_type)) > 0

  # If the machine type indicates a GPU is used, gather the count and type information
  accelerator_types = {
    "highgpu"  = "nvidia-tesla-a100"
    "megagpu"  = "nvidia-tesla-a100"
    "ultragpu" = "nvidia-a100-80gb"
  }
  generated_guest_accelerator = try([{
    type  = lookup(local.accelerator_types, regex("a2-([A-Za-z]+)-", var.machine_type)[0], ""),
    count = one(regex("a2-[A-Za-z]+-(\\d+)", var.machine_type)),
  }], [])

  # Set the guest_accelerator to the user defined value if supplied, otherwise
  # use the locally generated accelerator list.
  guest_accelerator = local.is_a2_vm ? coalescelist(
    var.guest_accelerator,
    local.generated_guest_accelerator,
  ) : var.guest_accelerator
}
