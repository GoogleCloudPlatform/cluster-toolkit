# Copyright 2023 Google LLC
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

variable "trigger" {
  description = "View of google_cloudbuild_trigger resource"
  type = object({
    name    = string
    id      = string
    project = string
  })
}

variable "schedule" {
  description = "Describes the schedule on which the job will be executed."
  type        = string
}

variable "time_zone" {
  description = "Specifies the time zone to be used in interpreting schedule."
  type        = string
  default     = "Asia/Kolkata"
}

variable "retry_count" {
  description = "Number of times to retry a failed build"
  type        = number
  default     = 1
  validation {
    condition     = var.retry_count >= 0 && var.retry_count <= 5
    error_message = "var.retry_count cannot be negative or greater than 5"
  }
}
