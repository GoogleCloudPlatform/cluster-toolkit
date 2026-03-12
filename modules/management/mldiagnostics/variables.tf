# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

variable "project_id" {
  description = "The project ID that hosts the gke cluster."
  type        = string
}

variable "cluster_id" {
  description = "An identifier for the gke cluster resource with format projects/<project_id>/locations/<region>/clusters/<name>."
  type        = string
  nullable    = false
}
# Add a variable to enforce dependency ordering in Terraform
variable "gke_cluster_exists" {
  description = "A static flag that signals to downstream modules that a cluster has been created."
  type        = bool
  default     = false
}

variable "namespace" {
  description = "Namespace for mldiagnostics"
  type        = string
  default     = "gke-mldiagnostics"
}

variable "workload_manager_wait" {
  description = "Dependency to wait for workload manager installation"
  type        = any
  default     = null
}

variable "cert_manager" {
  description = "Install cert-manager"
  type = object({
    install = optional(bool, false)
  })
  default = {}
}

variable "mldiagnostics_webhook" {
  description = "Install mldiagnostics webhook"
  type = object({
    install = optional(bool, false)
  })
  default = {}
}

variable "mldiagnostics_connection_operator" {
  description = "Install mldiagnostics connection operator"
  type = object({
    install = optional(bool, false)
  })
  default = {}
}
