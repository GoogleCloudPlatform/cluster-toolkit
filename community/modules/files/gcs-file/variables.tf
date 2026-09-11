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
  description = "Target GCS bucket name where the file will be stored."
  type        = string
  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9._-]{1,61}[a-z0-9]$", var.bucket_name))
    error_message = "bucket_name must be a valid GCS bucket name."
  }
}

variable "object_path" {
  description = "GCS object path (e.g., 'config/variables.env' or 'scripts/setup.sh')."
  type        = string
  validation {
    condition     = length(trimspace(var.object_path)) > 0 && substr(var.object_path, 0, 1) != "/"
    error_message = "object_path must be a non-empty relative GCS object path."
  }
}

variable "content" {
  description = "Inline string content to upload to the GCS object. Mutually exclusive with source_path."
  type        = string
  default     = null
}

variable "source_path" {
  description = "Path to a local file to upload to the GCS object. Mutually exclusive with content."
  type        = string
  default     = null
}

variable "content_type" {
  description = "The Content-Type of the object (e.g., 'text/plain', 'application/json'). If omitted, it will be automatically inferred."
  type        = string
  default     = null
}
