/**
 * Copyright 2025 Google LLC
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
  # Safely check for the existence of a TPU topology.
  is_tpu = try(var.placement_policy.tpu_topology, null) != null && try(var.placement_policy.tpu_topology, "") != ""

  tpu_taint = local.is_tpu ? [{
    key    = "google.com/tpu"
    value  = "present"
    effect = "NO_SCHEDULE"
  }] : []

  # Exhaustive map of machine prefixes to GKE accelerator labels.
  tpu_accelerator_map = {
    "ct4p"  = "tpu-v4-podslice"      # TPU v4
    "ct5lp" = "tpu-v5-lite-podslice" # TPU v5e
    "ct5p"  = "tpu-v5p-slice"        # TPU v5p
    "ct6e"  = "tpu-v6e-slice"        # TPU v6e
  }

  # Map specific GCE machine types to the number of TPU chips per node (VM).
  # Based on public Google Cloud TPU documentation.
  tpu_chip_count_map = {
    # v4 - ct4p
    "ct4p-hightpu-4t" = 4

    # v5e - ct5lp
    "ct5lp-hightpu-1t" = 1
    "ct5lp-hightpu-4t" = 4
    "ct5lp-hightpu-8t" = 8

    # v5p - ct5p
    "ct5p-hightpu-1t" = 1
    "ct5p-hightpu-2t" = 2
    "ct5p-hightpu-4t" = 4

    # v6e - ct6e
    "ct6e-standard-1t" = 1
    "ct6e-standard-4t" = 4
    "ct6e-standard-8t" = 8
  }

  # Robustly extract the machine family prefix (e.g., "ct6e").
  tpu_machine_family   = local.is_tpu ? element(split("-", var.machine_type), 0) : ""
  tpu_accelerator_type = local.is_tpu ? lookup(local.tpu_accelerator_map, local.tpu_machine_family, null) : null
  tpu_chips_per_node   = local.is_tpu ? lookup(local.tpu_chip_count_map, var.machine_type, null) : null
}

terraform {
  required_version = ">= 1.3"
}
