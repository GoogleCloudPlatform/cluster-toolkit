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

variable "bucket_name" {
  description = "Name of the target Google Cloud Storage bucket."
  type        = string

  validation {
    condition     = !can(regex("^gs://", var.bucket_name))
    error_message = "The bucket_name must not start with 'gs://'."
  }
}

variable "repo_url" {
  description = "Default Git repository URL to clone files/directories from."
  type        = string
  default     = null
}

variable "repo_ref" {
  description = "Default Git branch, commit SHA, or tag to checkout. It is strongly recommended to pin this to a specific commit SHA or tag rather than a mutable branch name (e.g. 'main') so triggers_replace detects upstream updates."
  type        = string
  default     = "main"
}

variable "directories" {
  description = "List of directories to synchronize to the GCS bucket from a remote Git repository."
  type = list(object({
    source_path      = string
    destination_path = string
    repo_url         = optional(string)
    repo_ref         = optional(string)
    exclude          = optional(list(string), [".*Dockerfile$"])
  }))
  default = []

  validation {
    condition = alltrue([
      for d in var.directories :
      try(coalesce(d.repo_url, var.repo_url), null) != null
    ])
    error_message = "Each entry in directories must set repo_url (or inherit var.repo_url); use the gcs-objects module for local files and directories."
  }

  validation {
    condition = alltrue([
      for d in var.directories :
      trim(d.destination_path, "/") != ""
    ])
    error_message = "destination_path cannot be empty or root ('/'). To protect other bucket contents, syncing with delete-unmatched must be scoped to a subpath prefix."
  }
}

variable "files" {
  description = "List of individual files to synchronize to the GCS bucket from a remote Git repository."
  type = list(object({
    source_path      = string
    destination_path = string
    repo_url         = optional(string)
    repo_ref         = optional(string)
  }))
  default = []

  validation {
    condition = alltrue([
      for f in var.files :
      try(coalesce(f.repo_url, var.repo_url), null) != null
    ])
    error_message = "Each entry in files must set repo_url (or inherit var.repo_url); use the gcs-objects module for local files and directories."
  }
}
