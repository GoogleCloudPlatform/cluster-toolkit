# Copyright 2023 Google LLC
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

########## GENERAL 
variable "project_id" {
  type        = string
  description = "Project ID to create resources in."
}

variable "region" {
  type        = string
  description = "Region to create resources in."
}


variable "name" {
  type        = string
  description = "Cluster name, used for resource naming and slurm accounting."

  validation {
    condition     = can(regex("^[a-z](?:[a-z0-9]{0,9})$", var.name))
    error_message = "Variable 'name' must be a match of regex '^[a-z](?:[a-z0-9]{0,9})$'."
  }
}


variable "subnetwork_self_link" {
  type        = string
  description = "Subnet to deploy to."
}

########## BUCKET
# TODO

########## CONTROLLER: CLOUD  
variable "controller" {
  description = <<-EOD
    Controller configuration. DO NOT configure manually, use `controller` module instead.
    EOD
  type        = any
  default = {
    machine_type = "n1-standard-4"
    disk_type    = "pd-standard"
  }

  validation {
    condition     = !contains(["c3-:pd-standard", "h3-:pd-standard", "h3-:pd-ssd"], "${substr(var.controller.machine_type, 0, 3)}:${var.controller.disk_type}")
    error_message = "A disk_type=${var.controller.disk_type} cannot be used with machine_type=${var.controller.machine_type}."
  }
}

########## CONTROLLER: HYBRID 
# TODO

########## LOGIN 
# TODO

########## NODESETS
variable "nodeset" {
  description = <<-EOD
    Nodesets configuration. DO NOT configure manually, use `nodeset` module instead.
    EOD
  type        = list(any)
  default     = []
  validation {
    condition     = length(distinct([for x in var.nodeset : x.name])) == length(var.nodeset)
    error_message = "All nodesets must have a unique name."
  }
}

########## PARTITION
variable "partition" {
  description = <<-EOD
    Partitions configuration. DO NOT configure manually, use `partition` module instead.
    EOD
  type        = list(any)
  default     = []
}

########## SLURM: TODO

variable "debug_mode" {
  description = <<EOD
Developer debug mode:
- Do not create cluster resources.
- Output `debug` variable containing cluster configuration.
EOD
  type        = bool
  default     = false
}
