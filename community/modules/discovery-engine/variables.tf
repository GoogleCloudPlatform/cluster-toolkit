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

variable "project_id" {
  description = "GCP project ID."
  type        = string
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{4,28}[a-z0-9]$", var.project_id))
    error_message = "Project ID must be 6 to 30 characters long, start with a lowercase letter, end with a letter or digit, and contain only lowercase letters, digits, and hyphens."
  }
}

variable "location" {
  description = "Discovery Engine location (e.g., 'global')."
  type        = string
  default     = "global"
  validation {
    condition     = can(regex("^[a-z0-9-]+$", var.location))
    error_message = "Location must be a valid GCP location identifier (e.g. 'global', 'us', 'eu', 'us-central1')."
  }
}

variable "base_url" {
  description = "Base URL for the Discovery Engine API."
  type        = string
  default     = "discoveryengine.googleapis.com"
  validation {
    condition     = can(regex("^[a-zA-Z0-9.-]+$", var.base_url))
    error_message = "Base URL must be a valid URL."
  }
}

variable "collection" {
  description = "Discovery Engine collection ID (e.g., 'default_collection')."
  type        = string
  default     = "default_collection"
  validation {
    condition     = can(regex("^[a-zA-Z0-9_-]{1,63}$", var.collection))
    error_message = "Collection ID must be a non-empty string of up to 63 characters containing letters, numbers, hyphens, or underscores."
  }
}

variable "engine_id" {
  description = "Unique ID for the Discovery Engine chat assistant engine."
  type        = string
  validation {
    condition     = can(regex("^[a-z0-9]([a-z0-9-_]{0,61}[a-z0-9])?$", var.engine_id))
    error_message = "Engine ID must conform to (1-63 characters, lowercase letters, numbers, and hyphens, underscores, starting and ending with an alphanumeric character)."
  }
}

variable "assistant_id" {
  description = "Unique ID for the Assistant attached to the Discovery Engine."
  type        = string
  validation {
    condition     = can(regex("^[a-z0-9]([a-z0-9-_]{0,61}[a-z0-9])?$", var.assistant_id))
    error_message = "Assistant ID must conform to (1-63 characters, lowercase letters, numbers, and hyphens, underscores starting and ending with an alphanumeric character)."
  }
}

# tflint-ignore: terraform_unused_declarations
variable "module_instance_id" {
  description = "Unique ID of this module instance (automatically populated by gcluster)."
  type        = string
  default     = "discovery-engine-assistant"
}
