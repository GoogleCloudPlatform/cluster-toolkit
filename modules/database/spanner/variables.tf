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
  type        = string
  description = "Project ID for Spanner instance."
}

variable "instance_name" {
  type        = string
  description = "Name of the Spanner instance."
}

variable "config" {
  type        = string
  description = "The name of the instance's configuration."
  default     = "regional-us-central1"
}

variable "display_name" {
  type        = string
  description = "The descriptive name for this instance as it appears in UIs."
  default     = "Spanner Instance"
}

variable "processing_units" {
  type        = number
  description = "The number of processing units allocated to this instance."
  default     = 100
}

variable "labels" {
  type        = map(string)
  description = "Labels to apply to the Spanner instance."
  default     = {}
}

variable "edition" {
  type        = string
  description = "The edition of the Spanner instance (e.g., ENTERPRISE, ENTERPRISE_PLUS, or STANDARD)."
  default     = "STANDARD"
}

variable "databases" {
  type = map(object({
    name                = string
    deletion_protection = optional(bool, true)
    iam_members = optional(list(object({
      role   = string
      member = string
    })), [])
  }))
  description = "A map of databases to create. Keys are logical names."
  default     = {}
}
