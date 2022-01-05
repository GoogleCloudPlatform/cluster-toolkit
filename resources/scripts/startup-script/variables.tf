/**
 * Copyright 2021 Google LLC
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

variable "deployment_name" {
  description = "Name of the HPC deployment, used to name GCS bucket for startup scripts."
  type        = string
}

variable "region" {
  description = "The region to deploy to"
  type        = string
}


variable "debug_file" {
  description = "Path to an optional local to be written with 'startup_script_content'."
  type        = string
  default     = null
}

variable "runners" {
  description = <<EOT
    List of runners to run on remote VM.
    Runners can be of type ansible-local, shell or data.
    A runner must specify one of 'source' or 'content'.
    All runners must specify 'destination'. If 'destination' does not include a
    path, it will be copied in a temporary folder and deleted after running.
    Runners may also pass 'args', which will be passed as argument to shell runners only.
EOT
  type        = list(map(string))
  validation {
    condition = alltrue([
      for r in var.runners : contains(keys(r), "type")
    ])
    error_message = "All runners must declare a type."
  }
  validation {
    condition = alltrue([
      for r in var.runners : contains(keys(r), "destination")
    ])
    error_message = "All runners must declare a destination name (even without a path)."
  }
  validation {
    condition = alltrue([
      for r in var.runners : r["type"] == "ansible-local" || r["type"] == "shell" || r["type"] == "data"
    ])
    error_message = "The 'type' must be 'ansible-local', 'shell' or 'data'."
  }
  validation {
    condition = alltrue([
      for r in var.runners :
      (contains(keys(r), "content") && !contains(keys(r), "source")) ||
      (!contains(keys(r), "content") && contains(keys(r), "source"))
    ])
    error_message = "A runner must specify one of 'content' or 'source file', but not both."
  }
  default = []
}
