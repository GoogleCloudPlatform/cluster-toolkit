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

output "is_tpu" {
  description = "Boolean value indicating if the node pool is for TPUs."
  value       = local.is_tpu
}

output "tpu_accelerator_type" {
  description = "The label value for the TPU accelerator type (e.g., 'tpu-v6e-slice')."
  value       = local.tpu_accelerator_type
}

output "tpu_topology" {
  description = "The topology of the TPU slice (e.g., '4x4')."
  value       = local.is_tpu ? var.placement_policy.tpu_topology : null
}

output "tpu_chips_per_node" {
  description = "The number of TPU chips on each node in the pool."
  value       = local.tpu_chips_per_node
}

output "tpu_taint" {
  description = "A list containing the standard TPU taint object if the node pool is for TPUs."
  value       = local.tpu_taint
}
