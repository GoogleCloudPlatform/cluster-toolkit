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

variable "project_id" {
  description = "Provide Project name "
  type        = string
  default     = null
}

variable "labels" {
  description = "Labels to add to supporting resources. Key-value pairs."
  type        = map(string)
}

variable "network_name" {
  description = "Name of the VPC Network."
  type        = string
}

variable "vpc_address_ip_range" {
  description = "The IP address or beginning of the address range allocated for the private service access."
  type        = string
  default     = null
}

variable "vpc_peering_address_ip_range" {
  description = "Private Service Access - VPC Peering Address Range to set."
  type        = string
  default     = null
}

variable "vpc_peering_address_prefix" {
  description = "Private Service Access - VPC Peering Address Prefix to set."
  type        = string
  default     = "24"
}

variable "vpc_peering_address_type" {
  description = "Private Service Access - VPC Peering Address Type"
  type        = string
  default     = "INTERNAL"
}

variable "vpc_peering_ip_version" {
  description = "Private Service Access - VPC Peering IP Version"
  type        = string
  default     = "IPV4"
}

variable "vpc_peering_purpose" {
  description = "Private Service Access - VPC Peering Purpose"
  type        = string
  default     = "VPC_PEERING"
}

variable "vpc_connection_peering_service" {
  description = "VPC Connection Service API"
  type        = string
  default     = "servicenetworking.googleapis.com"
}

variable "netapp_connection_peering_service" {
  description = "NetApp VPC Connection Peering Service API"
  type        = string
  default     = "netapp.servicenetworking.goog"
}