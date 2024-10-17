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

## Required variables:
#  guest_accelerator
#  machine_type

locals {
  # example state; terraform will ignore diffs if last element of URL matches
  # guest_accelerator = [
  #   {
  #     count = 1
  #     type  = "https://www.googleapis.com/compute/beta/projects/PROJECT/zones/ZONE/acceleratorTypes/nvidia-tesla-a100"
  #   },
  # ]
  accelerator_machines = {
    "a2-highgpu-1g"  = { type = "nvidia-tesla-a100", count = 1 },
    "a2-highgpu-2g"  = { type = "nvidia-tesla-a100", count = 2 },
    "a2-highgpu-4g"  = { type = "nvidia-tesla-a100", count = 4 },
    "a2-highgpu-8g"  = { type = "nvidia-tesla-a100", count = 8 },
    "a2-megagpu-16g" = { type = "nvidia-tesla-a100", count = 16 },
    "a2-ultragpu-1g" = { type = "nvidia-a100-80gb", count = 1 },
    "a2-ultragpu-2g" = { type = "nvidia-a100-80gb", count = 2 },
    "a2-ultragpu-4g" = { type = "nvidia-a100-80gb", count = 4 },
    "a2-ultragpu-8g" = { type = "nvidia-a100-80gb", count = 8 },
    "a3-highgpu-8g"  = { type = "nvidia-h100-80gb", count = 8 },
    "a3-megagpu-8g"  = { type = "nvidia-h100-mega-80gb", count = 8 },
    "g2-standard-4"  = { type = "nvidia-l4", count = 1 },
    "g2-standard-8"  = { type = "nvidia-l4", count = 1 },
    "g2-standard-12" = { type = "nvidia-l4", count = 1 },
    "g2-standard-16" = { type = "nvidia-l4", count = 1 },
    "g2-standard-24" = { type = "nvidia-l4", count = 2 },
    "g2-standard-32" = { type = "nvidia-l4", count = 1 },
    "g2-standard-48" = { type = "nvidia-l4", count = 4 },
    "g2-standard-96" = { type = "nvidia-l4", count = 8 },
  }
  generated_guest_accelerator = try([local.accelerator_machines[var.machine_type]], [{ count = 0, type = "" }])

  # Select in priority order:
  # (1) var.guest_accelerator if not empty
  # (2) local.generated_guest_accelerator if not empty
  # (3) default to empty list if both are empty
  guest_accelerator = try(coalescelist(var.guest_accelerator, local.generated_guest_accelerator), [{ count = 0, type = "" }])
}
