# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

variable "project_id" {
  description = "Project containing the GKE cluster and managed service accounts."
  type        = string
}

variable "deployment_name" {
  description = "Unique deployment name used to name managed service accounts."
  type        = string

  validation {
    condition     = !var.create_service_accounts || can(regex("^[a-z][-a-z0-9]{4,28}[a-z0-9]$", "${var.deployment_name}-nodes"))
    error_message = "deployment_name must produce a valid service account ID when suffixed with -nodes or -proxy (6 to 30 lowercase letters, digits, or dashes, starting with a letter)."
  }
}

variable "namespace" {
  description = "Kubernetes namespace containing the agentgateway-proxy service account."
  type        = string
}

variable "create_service_accounts" {
  description = "Create service accounts and IAM bindings. When false, use existing accounts without IAM writes."
  type        = bool
  default     = true
  nullable    = false
}

variable "node_service_account_email" {
  description = "Existing node service account. Required when create_service_accounts is false; otherwise leave empty."
  type        = string
  default     = ""
  nullable    = false

  validation {
    condition     = var.create_service_accounts ? var.node_service_account_email == "" : can(regex("^[^@[:space:]]+@[^@[:space:]]+\\.gserviceaccount\\.com$", var.node_service_account_email))
    error_message = "Set node_service_account_email to an existing Google service account email when create_service_accounts=false; leave it empty when true."
  }
}

variable "proxy_service_account_email" {
  description = "Existing proxy service account. Required when create_service_accounts is false; otherwise leave empty."
  type        = string
  default     = ""
  nullable    = false

  validation {
    condition     = var.create_service_accounts ? var.proxy_service_account_email == "" : can(regex("^[^@[:space:]]+@[^@[:space:]]+\\.gserviceaccount\\.com$", var.proxy_service_account_email))
    error_message = "Set proxy_service_account_email to an existing Google service account email when create_service_accounts=false; leave it empty when true."
  }
}
