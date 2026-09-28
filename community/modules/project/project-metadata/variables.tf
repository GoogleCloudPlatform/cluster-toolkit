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

variable "project_id" {
  description = "GCP project ID where compute metadata will be set."
  type        = string
}

variable "metadata" {
  description = "Project compute metadata items to set as a list of key-value objects."
  type = list(object({
    key   = string
    value = string
  }))

  validation {
    condition     = length(distinct([for m in var.metadata : m.key])) == length(var.metadata)
    error_message = "metadata keys must be unique."
  }
}

variable "deletion_policy" {
  description = "The deletion policy for the metadata items. Can be 'DELETE' (removes the key from project metadata on destroy) or 'ABANDON' (leaves the metadata key in place on destroy)."
  type        = string
  default     = "DELETE"

  validation {
    condition     = contains(["DELETE", "ABANDON"], var.deletion_policy)
    error_message = "deletion_policy must be either 'DELETE' or 'ABANDON'."
  }
}
