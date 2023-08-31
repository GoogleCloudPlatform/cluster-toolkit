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

variable "project_id" {
  description = "Project in which the HPC deployment will be created."
  type        = string
}

variable "region" {
  type        = string
  description = "Region to create resources in."
}

variable "name" {
  description = "Name of the node set."
  type        = string
}

variable "node_conf" {
  description = "Map of Slurm node line configuration."
  type        = map(any)
  default     = {}
}

variable "node_count_static" {
  type    = number
  default = 0
}

variable "node_count_dynamic_max" {
  type    = number
  default = 1
}

// Instance properties

variable "machine_type" {
  description = "Compute Platform machine type to use for this partition compute nodes."
  type        = string
  default     = "c2-standard-60"
}

variable "zones" {
  description = <<-EOD
    Zones in which to allow creation of nodes. Google Cloud
    will find zone based on availability, quota and reservations.
    EOD
  type        = set(string)
  default     = []

  validation {
    condition = alltrue([
      for x in var.zones : length(regexall("^[a-z]+-[a-z]+[0-9]-[a-z]$", x)) > 0
    ])
    error_message = "A value in var.zones is not a valid zone (example: us-central1-f)."
  }
}

variable "zone_target_shape" {
  description = <<EOD
Strategy for distributing VMs across zones in a region.
ANY
  GCE picks zones for creating VM instances to fulfill the requested number of VMs
  within present resource constraints and to maximize utilization of unused zonal
  reservations.
ANY_SINGLE_ZONE (default)
  GCE always selects a single zone for all the VMs, optimizing for resource quotas,
  available reservations and general capacity.
BALANCED
  GCE prioritizes acquisition of resources, scheduling VMs in zones where resources
  are available while distributing VMs as evenly as possible across allowed zones
  to minimize the impact of zonal failure.
EOD
  type        = string
  default     = "ANY_SINGLE_ZONE"
  validation {
    condition     = contains(["ANY", "ANY_SINGLE_ZONE", "BALANCED"], var.zone_target_shape)
    error_message = "Allowed values for zone_target_shape are \"ANY\", \"ANY_SINGLE_ZONE\", or \"BALANCED\"."
  }
}

variable "enable_placement" { // REVIEWER_NOTE: moved down from partition
  description = <<-EOD
    Enables compact placement policy for instances.
    Use compact policies when you want VMs to be located close to each other for low network latency between the VMs.
    See https://cloud.google.com/compute/docs/instances/define-instance-placement for details.
  EOD
  type        = bool
  default     = true
}

// IMPORTANT: See `variables_instance.tf` for more instance variables.
// The majority of instance properties are shared by nodeset, controller, and login nodes.
// For the sake of consistenct we define them in identicaly replicated `variables_instance.tf`.

