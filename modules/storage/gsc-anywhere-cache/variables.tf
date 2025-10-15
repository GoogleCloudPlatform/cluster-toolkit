# Copyright 2025 "Google LLC"
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

variable "bucket_name" {
  type        = string
  description = "The name of the bucket."
}

variable "caches" {
  type = list(object({
    zone             = string
    ttl              = optional(string, "86400s")
    admission_policy = optional(string, "admit-on-first-miss")
  }))
  description = "A list of Anywhere Cache configurations."
  default     = []

  validation {
    condition = alltrue([
      for cache in var.caches :
      contains(
        ["admit-on-first-miss", "admit-on-second-miss"],
        coalesce(cache.admission_policy, "admit-on-first-miss")
      )
    ])
    # MODIFIED ERROR MESSAGE:
    error_message = "DEBUG STALE CHECK: Allowed policies are 'admit-on-first-miss' or 'admit-on-second-miss'."
  }
}
