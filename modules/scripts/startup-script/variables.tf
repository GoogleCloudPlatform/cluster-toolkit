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

variable "project_id" {
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "deployment_name" {
  description = "Name of the HPC deployment, used to name GCS bucket for startup scripts."
  type        = string
}

variable "region" {
  description = "The region to deploy to"
  type        = string
}


variable "gcs_bucket_path" {
  description = "The GCS path for storage bucket and the object."
  type        = string
  default     = null
}

variable "debug_file" {
  description = "Path to an optional local to be written with 'startup_script'."
  type        = string
  default     = null
}

variable "labels" {
  description = "Labels for the created GCS bucket. List key, value pairs."
  type        = any
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
    condition     = length(distinct([for r in var.runners : r["destination"]])) == length(var.runners)
    error_message = "All startup-script runners must have a unique destination."
  }
  validation {
    condition = alltrue([
      for r in var.runners : r["type"] == "ansible-local" || r["type"] == "shell" || r["type"] == "data"
    ])
    error_message = "The 'type' must be 'ansible-local', 'shell' or 'data'."
  }
  # this validation tests that exactly 1 or other of source/content have been
  # set to anything (including null)
  validation {
    condition = alltrue([
      for r in var.runners :
      can(r["content"]) != can(r["source"])
    ])
    error_message = "A runner must specify either 'content' or 'source', but never both."
  }
  # this validation tests that at least 1 of source/content are non-null
  # can fail either by not having been set all or by being set to null
  validation {
    condition = alltrue([
      for r in var.runners :
      lookup(r, "content", lookup(r, "source", null)) != null
    ])
    error_message = "A runner must specify a non-null 'content' or 'source'."
  }
  default = []
}

variable "prepend_ansible_installer" {
  description = "Prepend Ansible installation script if any of the specified runners are of type ansible-local"
  type        = bool
  default     = true
}
