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

variable "machine_type" {
  description = "Machine type to use for the instance creation"
  type        = string
}

variable "guest_accelerator" {
  description = "List of the type and count of accelerator cards attached to the instance."
  type = list(object({
    type  = string
    count = number
    gpu_driver_installation_config = optional(object({
      gpu_driver_version = string
    }), { gpu_driver_version = "DEFAULT" })
    gpu_partition_size = optional(string)
    gpu_sharing_config = optional(object({
      gpu_sharing_strategy       = string
      max_shared_clients_per_gpu = number
    }))
  }))
  default = []
}

variable "machine_configs" {
  description = "Definition of GCE machine types and counts"
  type        = any
  default     = {}
}
