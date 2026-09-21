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
  description = "GCP project ID"
  type        = string
}

variable "service" {
  description = "The GCP service API for which the service agent is to be created"
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]*(\\.[a-z0-9-]+)*\\.googleapis\\.com$", var.service))
    error_message = "var.service must be a fully qualified Google API service name ending in .googleapis.com, for example \"batch.googleapis.com\"."
  }
}

variable "enable_service" {
  description = "Enable the target API before generating the service identity. Set to false if the API is already enabled and the deployment service account lacks serviceusage.serviceUsageAdmin."
  type        = bool
  default     = true
}

variable "disable_on_destroy" {
  description = "Disable the service on destroy if it was enabled (or already enabled) during apply (default: false)"
  type        = bool
  default     = false
}
