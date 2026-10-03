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
}

variable "location" {
  description = "Discovery Engine location (e.g., 'global', 'us', 'eu', or an allowlisted in-country location). See [Gemini Enterprise locations](https://cloud.google.com/gemini/enterprise/docs/locations) and [Agent Search locations](https://cloud.google.com/generative-ai-app-builder/docs/locations)."
  type        = string
  default     = "global"
  validation {
    condition     = contains(["global", "us", "eu", "ca", "in", "sg", "asia-northeast1", "europe-west2"], var.location)
    error_message = "location must be a supported Discovery Engine location (global, us, eu, ca, in, sg, asia-northeast1, europe-west2). See https://cloud.google.com/gemini/enterprise/docs/locations for details."
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
    condition     = can(regex("^[a-z0-9]([a-z0-9-_]{0,58}[a-z0-9])?$", var.engine_id))
    error_message = "Engine ID must conform to (1-60 characters, lowercase letters, numbers, and hyphens, underscores, starting and ending with an alphanumeric character)."
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
