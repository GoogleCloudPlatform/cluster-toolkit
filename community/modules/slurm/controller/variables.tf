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

###########
# GENERAL #
###########

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

#------------------
# Controller
#------------------

variable "machine_type" {
  type        = string
  description = "Machine type to create."
  default     = "c2-standard-4"
}

variable "zone" {
  type        = string
  description = <<EOD
Zone where the instances should be created. If not specified, instances will be
spread across available zones in the region.
EOD
  default     = null
}

variable "network_ip" {
  type        = string
  description = "Private IP address to assign to the instance if desired."
  default     = ""
}

variable "static_ip" {
  type        = string
  description = "Static IP for controller instance."
  default     = null
}

// IMPORTANT: See `variables_instance.tf` for more instance variables.
// The majority of instance properties are shared by nodeset, controller, and login nodes.
// For the sake of consistenct we define them in identicaly replicated `variables_instance.tf`.


variable "nodeset" {
  type = list(any) # TODO: move partition/variables.nodeset to separate file and reuse it here.
}

variable "partition" {
  type = list(any) # TODO: unwrap?
}