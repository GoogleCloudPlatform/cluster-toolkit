/**
 * Copyright 2022 Google LLC
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

# Most variables have been sourced and modified from the SchedMD/slurm-gcp
# github repository: https://github.com/GoogleCloudPlatform/slurm-gcp/tree/v5

variable "deployment_name" {
  description = "Name of the deployment."
  type        = string
}

variable "slurm_cluster_name" {
  type        = string
  description = "Cluster name, used for resource naming and slurm accounting. If not provided it will default to the first 8 characters of the deployment name (removing any invalid characters)."
  default     = null
}

variable "project_id" {
  description = "Project in which the HPC deployment will be created."
  type        = string
}

variable "region" {
  description = "The default region for Cloud resources."
  type        = string
}

variable "partition_name" {
  description = "The name of the slurm partition."
  type        = string

  validation {
    condition     = can(regex("^[a-z](?:[a-z0-9]{0,6})$", var.partition_name))
    error_message = "Variable 'partition_name' must be composed of only alphanumeric characters, start with a letter and be 7 characters or less. Regexp: '^[a-z](?:[a-z0-9]{0,6})$'."
  }
}

variable "partition_conf" {
  description = <<-EOD
    Slurm partition configuration as a map.
    See https://slurm.schedmd.com/slurm.conf.html#SECTION_PARTITION-CONFIGURATION
    EOD
  type        = map(string)
  default     = {}
}

variable "is_default" {
  description = <<-EOD
    Sets this partition as the default partition by updating the partition_conf.
    If "Default" is already set in partition_conf, this variable will have no effect.
    EOD
  type        = bool
  default     = false
}

variable "subnetwork_self_link" {
  type        = string
  description = "Subnet to deploy to."
  default     = null
}

variable "subnetwork_project" {
  description = "The project the subnetwork belongs to."
  type        = string
  default     = ""
}

variable "exclusive" {
  description = "Exclusive job access to nodes."
  type        = bool
  default     = true
}

variable "partition_feature" {
  description = <<-EOD
    Any nodes with this feature will automatically be put into this partition.

    NOTE: meant to be used for external dynamic nodes that register.
  EOD
  type        = string
}
