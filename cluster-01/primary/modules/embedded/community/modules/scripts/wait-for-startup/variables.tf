/**
 * Copyright 2022 Google LLC
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

variable "instance_name" {
  description = "Name of the instance we are waiting for (can be null if 'instance_names' is not empty)"
  type        = string
  default     = null
}

variable "instance_names" {
  description = "A list of instance names we are waiting for, in addition to the one mentioned in 'instance_name' (if any)"
  type        = list(string)
  default     = []
}

variable "zone" {
  description = "The GCP zone where the instance is running"
  type        = string
}

variable "project_id" {
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "timeout" {
  description = "Timeout in seconds"
  type        = number
  default     = 1200
  validation {
    condition     = var.timeout >= 0
    error_message = "The timeout should be non-negative"
  }
}

variable "gcloud_path_override" {
  description = "Directory of the gcloud executable to be used during cleanup"
  type        = string
  default     = ""
  nullable    = false
}
