# Copyright 2024 "Google LLC"
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

variable "name" {
  description = <<-EOD
    Name of the nodeset. Automatically populated by the module id if not set.
    If setting manually, ensure a unique value across all nodesets.
    EOD
  type        = string
}

variable "node_feature" {
  description = "Nodeset feature for dynamic registration. Defaults to nodeset name."
  type        = string
  default     = null
}

variable "slurm_cluster_name" {
  description = "Name of the Slurm cluster."
  type        = string
}

variable "slurm_bucket_path" {
  description = "GCS Bucket URI of Slurm cluster file storage."
  type        = string
}

variable "project_id" {
  description = "Project ID to create resources in."
  type        = string
}


variable "region" {
  description = "The default region for Cloud resources."
  type        = string
}

variable "zone" {
  description = "Zone in which to create compute VMs. Additional zones in the same region can be specified in var.zones."
  type        = string
}

variable "labels" {
  description = "Labels to add to instances. Key-value pairs."
  type        = map(string)
  default     = {}
}


variable "target_size" {
  description = "The target number of running instances for this managed instance group."
  type        = number
}

variable "machine_type" {
  description = "Machine type to create."
  type        = string
  default     = "n1-standard-1"
}

variable "subnetwork_self_link" {
  description = "Subnet to deploy to."
  type        = string
}
