# Copyright 2025 "Google LLC"
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
  description = "The project ID that hosts the GKE cluster."
  type        = string
}

variable "cluster_id" {
  description = "An identifier for the GKE cluster resource with format projects/<project_id>/locations/<region>/clusters/<name>."
  type        = string
  nullable    = false
}

variable "cert_manager_values" {
  description = "Value overrides for the Cert Manager release"
  type        = any
  default = {
    crds = {
      enabled = true
    }
  }
}

variable "slurm_operator_values" {
  description = "Value overrides for the Slinky release"
  type        = any
  default     = {}
}

variable "slurm_values" {
  description = "Value overrides for the Slurm release"
  type        = any
  default     = {}
}
