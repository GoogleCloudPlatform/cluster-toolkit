/**
 * Copyright 2024 Google LLC
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
  description = "Project in which the HPC deployment will be created."
  type        = string
}

variable "deployment_name" {
  description = "Name of the HPC deployment."
  type        = string
}

variable "daos_agent_config" {
  description = "Additional configuration to be added to daos_config.yml"
  type        = string
  default     = ""
  nullable    = false
}

variable "dfuse_environment" {
  description = "Additional environment variables for DFuse process"
  type        = map(string)
  default     = {}
  nullable    = false
}

variable "name" {
  description = "Name of parallelstore instance."
  type        = string
  default     = null
}

variable "zone" {
  description = "Location for parallelstore instance."
  type        = string
}

variable "size_gb" {
  description = "Storage size of the parallelstore instance in GB."
  type        = number
  default     = 12000
}

variable "labels" {
  description = "Labels to add to parallel store instance."
  type        = map(string)
  default     = {}
}

variable "local_mount" {
  description = "The mount point where the contents of the device may be accessed after mounting."
  type        = string
  default     = "/parallelstore"
}

variable "mount_options" {
  description = "Options describing various aspects of the parallelstore instance."
  type        = string
  default     = "disable-wb-cache,thread-count=16,eq-count=8"
}

variable "private_vpc_connection_peering" {
  description = <<-EOT
    The name of the VPC Network peering connection.
    If using new VPC, please use community/modules/network/private-service-access to create private-service-access and
    If using existing VPC with private-service-access enabled, set this manually."
    EOT
  type        = string
}

variable "network_id" {
  description = <<-EOT
    The ID of the GCE VPC network to which the instance is connected given in the format:
    `projects/<project_id>/global/networks/<network_name>`"
    EOT
  type        = string
  validation {
    condition     = length(split("/", var.network_id)) == 5
    error_message = "The network id must be provided in the following format: projects/<project_id>/global/networks/<network_name>."
  }
}

variable "import_gcs_bucket_uri" {
  description = "The name of the GCS bucket to import data from to parallelstore."
  type        = string
  default     = null
}

variable "import_destination_path" {
  description = "The name of local path to import data on parallelstore instance from GCS bucket."
  type        = string
  default     = null
}

variable "file_stripe" {
  description = "The parallelstore stripe level for files."
  type        = string
  default     = null
  validation {
    condition = var.file_stripe == null ? true : contains([
      "FILE_STRIPE_LEVEL_UNSPECIFIED",
      "FILE_STRIPE_LEVEL_MIN",
      "FILE_STRIPE_LEVEL_BALANCED",
      "FILE_STRIPE_LEVEL_MAX",
    ], var.file_stripe)
    error_message = "var.file_stripe must be set to \"FILE_STRIPE_LEVEL_UNSPECIFIED\", \"FILE_STRIPE_LEVEL_MIN\", \"FILE_STRIPE_LEVEL_BALANCED\",  or \"FILE_STRIPE_LEVEL_MAX\""
  }
}

variable "directory_stripe" {
  description = "The parallelstore stripe level for directories."
  type        = string
  default     = null
  validation {
    condition = var.directory_stripe == null ? true : contains([
      "DIRECTORY_STRIPE_LEVEL_UNSPECIFIED",
      "DIRECTORY_STRIPE_LEVEL_MIN",
      "DIRECTORY_STRIPE_LEVEL_BALANCED",
      "DIRECTORY_STRIPE_LEVEL_MAX",
    ], var.directory_stripe)
    error_message = "var.directory_stripe must be set to \"DIRECTORY_STRIPE_LEVEL_UNSPECIFIED\", \"DIRECTORY_STRIPE_LEVEL_MIN\", \"DIRECTORY_STRIPE_LEVEL_BALANCED\",  or \"DIRECTORY_STRIPE_LEVEL_MAX\""
  }
}
