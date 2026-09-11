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
  description = "Default Git repository URL to clone files/directories from. If omitted or null, local filesystem paths are used by default."
  type        = string
  default     = null
}

variable "repo_ref" {
  description = "Default Git branch, commit, or tag to checkout when using Git repositories."
  type        = string
  default     = "main"
}

variable "directories" {
  description = "List of directories to stage/sync to the GCS bucket. Each entry defines a source directory (local or remote Git) and a destination path in the bucket."
  type = list(object({
    source_path      = string
    destination_path = string
    repo_url         = optional(string)
    repo_ref         = optional(string)
    exclude          = optional(list(string), [".*Dockerfile$"])
  }))
  default = []
}

variable "files" {
  description = "List of individual files to stage/upload to the GCS bucket. Each entry defines a source file (local or remote Git) and a destination object path in the bucket."
  type = list(object({
    source_path      = string
    destination_path = string
    repo_url         = optional(string)
    repo_ref         = optional(string)
  }))
  default = []
}
