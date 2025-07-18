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

variable "address" {
  description = "The IP address or beginning of the address range allocated for the private service access."
  type        = string
  default     = null
}

variable "network_id" {
  description = <<-EOT
    The ID of the GCE VPC network to configure private service Access.:
    `projects/<project_id>/global/networks/<network_name>`"
    EOT
  type        = string
  validation {
    condition     = length(split("/", var.network_id)) == 5
    error_message = "The network id must be provided in the following format: projects/<project_id>/global/networks/<network_name>."
  }
}

variable "labels" {
  description = "Labels to add to supporting resources. Key-value pairs."
  type        = map(string)
}

variable "prefix_length" {
  description = "The prefix length of the IP range allocated for the private service access."
  type        = number
  default     = 16
}

variable "project_id" {
  description = "ID of project in which Private Service Access will be created."
  type        = string
}
