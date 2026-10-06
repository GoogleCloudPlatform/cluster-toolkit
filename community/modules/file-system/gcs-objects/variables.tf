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
  description = "Target Google Cloud Storage bucket name where objects will be stored."
  type        = string
}

variable "files" {
  description = "Individual files to upload, from inline content or a local path."
  type = list(object({
    destination_path = string
    source_path      = optional(string)
    content          = optional(string)
    content_type     = optional(string)
  }))
  default = []
  validation {
    condition = alltrue([
      for f in var.files :
      (f.content != null) != (f.source_path != null)
    ])
    error_message = "Each entry in files must set exactly one of content or source_path."
  }
}

variable "directories" {
  description = "Local directories to upload recursively. Paths are resolved relative to the deployment root directory (typically staged via $(ghpc_stage(...))). Note that patterns in 'exclude' are matched as RE2 regular expressions against the relative file path, not shell globs."
  type = list(object({
    destination_path = string
    source_path      = string
    exclude          = optional(list(string), [])
  }))
  default = []
}
