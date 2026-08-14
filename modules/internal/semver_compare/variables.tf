# Copyright 2026 "Google LLC"
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

variable "current_version" {
  type        = string
  description = "The version string to evaluate (e.g. 1.35.2-gke, v0.15.2, sha256-123)."
}

variable "minimum_version" {
  type        = string
  description = "The minimum required version (e.g. 1.35.0)."

  validation {
    condition     = can(regex("^[vV]?([0-9]+)(?:\\.([0-9]+))?(?:\\.([0-9]+))?(?:-gke\\.([0-9]+))?(?:[-+].*)?$", var.minimum_version))
    error_message = "The minimum_version must be a valid major.minor.patch[-gke.X] string."
  }
}
