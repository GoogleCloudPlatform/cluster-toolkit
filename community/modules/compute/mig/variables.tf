# Copyright 2024 Google LLC
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
  description = "Project in which the MIG will be created"
  type        = string
}

variable "deployment_name" {
  description = "Name of the deployment, will be used to name MIG if `var.name` is not provided"
  type        = string
}

variable "labels" {
  description = "Labels to add to the MIG"
  type        = map(string)
}

variable "zone" {
  description = "Compute Platform zone. Required, currently only zonal MIGs are supported"
  type        = string
}


variable "versions" {
  description = <<-EOD
    Application versions managed by this instance group. Each version deals with a specific instance template
    EOD
  type = list(object({
    name              = string
    instance_template = string
    target_size = optional(object({
      fixed   = optional(number)
      percent = optional(number)
    }))
  }))

  validation {
    condition     = length(var.versions) > 0
    error_message = "At least one version must be provided"
  }

}


variable "ghpc_module_id" {
  description = "Internal GHPC field, do not set this value"
  type        = string
  default     = null
}

variable "name" {
  description = "Name of the MIG. If not provided, will be generated from `var.deployment_name`"
  type        = string
  default     = null
}

variable "base_instance_name" {
  description = "Base name for the instances in the MIG"
  type        = string
  default     = null
}


variable "target_size" {
  description = "Target number of instances in the MIG"
  type        = number
  default     = 0
}

variable "wait_for_instances" {
  description = "Whether to wait for all instances to be created/updated before returning"
  type        = bool
  default     = false
}
