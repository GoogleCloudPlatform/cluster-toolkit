# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

variable "project_id" {
  description = "Project containing the Agent Platform publisher model usage."
  type        = string
  nullable    = false
}

variable "deployment_name" {
  validation {
    condition     = can(regex("^[a-zA-Z0-9_.-]{1,56}$", var.deployment_name))
    error_message = "deployment_name must contain 1–56 letters, digits, underscores, periods or hyphens; the generated role ID adds _predict and replaces hyphens with underscores."
  }
  description = "Unique deployment name, used to name the custom role."
  type        = string
  nullable    = false
}

variable "service_account_email" {
  description = "Google service account granted prediction access."
  type        = string
  nullable    = false
}
