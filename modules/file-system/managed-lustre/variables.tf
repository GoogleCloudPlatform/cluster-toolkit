/**
 * Copyright 2025 Google LLC
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
  description = "ID of project in which Lustre instance will be created."
  type        = string
}

variable "description" {
  description = "Description of the created Lustre instance."
  type        = string
  default     = "Lustre Instance"
}

variable "deployment_name" {
  description = "Name of the HPC deployment, used as name of the Lustre instance if no name is specified."
  type        = string
}

variable "zone" {
  description = "Location for the Lustre instance."
  type        = string
}

variable "name" {
  description = "Name of the Lustre instance"
  type        = string
}

variable "network_id" {
  description = <<-EOT
    The ID of the GCE VPC network to which the instance is connected given in the format:
    `projects/<project_id>/global/networks/<network_name>`"
    EOT
  type        = string
  nullable    = false
  validation {
    condition     = length(split("/", var.network_id)) == 5
    error_message = "The network id must be provided in the following format: projects/<project_id>/global/networks/<network_name>."
  }
}

variable "network_self_link" {
  description = "Network self-link this instance will be on, required for checking private service access"
  type        = string
  nullable    = false
}

variable "remote_mount" {
  description = "Remote mount point of the Managed Lustre instance"
  type        = string
  nullable    = false
}

variable "local_mount" {
  description = "Local mount point for the Managed Lustre instance."
  type        = string
  default     = "/shared"
}

variable "size_gib" {
  description = "Storage size of the Managed Lustre instance in GB. See https://cloud.google.com/managed-lustre/docs/create-instance for limitations"
  type        = number
  default     = 18000
}

variable "labels" {
  description = "Labels to add to the Managed Lustre instance. Key-value pairs."
  type        = map(string)
}

variable "mount_options" {
  description = "Mounting options for the file system."
  type        = string
  default     = "defaults,_netdev"
}

variable "private_vpc_connection_peering" {
  description = <<-EOT
    The name of the VPC Network peering connection.
    If using new VPC, please use community/modules/network/private-service-access to create private-service-access and
    If using existing VPC with private-service-access enabled, set this manually."
    EOT
  type        = string
  nullable    = false
}
