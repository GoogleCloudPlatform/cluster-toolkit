/**
 * Copyright 2021 Google LLC
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
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "network_name" {
  description = "The name of the network to be created (defaults to \"var.deployment_name-net\")"
  type        = string
  default     = null
}

variable "region" {
  description = "The default region for Cloud resources"
  type        = string
}

variable "deployment_name" {
  description = "The name of the current deployment"
  type        = string
}

variable "network_address_range" {
  description = "IP address range (CIDR) for global network"
  type        = string
  default     = "10.0.0.0/9"

  validation {
    condition     = can(cidrhost(var.network_address_range, 0))
    error_message = "IP address range must be in CIDR format."
  }
}


# the default will create a subnetwork in var.region with the settings noted
variable "primary_subnetwork" {
  description = <<EOT
  Primary (default) subnetwork in which to create resources.

  name           (string, required, Name of subnet)
  region         (string, ignored, will be replaced by var.region)
  private_access (bool, optional, Enable Private Access on subnetwork)
  flow_logs      (map(string), optional, Configure Flow Logs see
                  terraform-google-network module for complete settings)
  description    (string, optional, Description of Network)
  purpose        (string, optional, related to Load Balancing)
  role           (string, optional, related to Load Balancing)
  EOT
  type        = map(string)
  default = {
    name           = "primary-subnetwork"
    description    = "Primary Subnetwork"
    new_bits       = 15
    private_access = true
    flow_logs      = false
  }
}

variable "additional_subnetworks" {
  description = <<EOT
  List of additional subnetworks in which to create resources.

  name           (string, required, Name of subnet; must be unique in region)
  region         (string, required)
  private_access (bool, optional, Enable Private Access on subnetwork)
  flow_logs      (map(string), optional, Configure Flow Logs see
                  terraform-google-network module for complete settings)
  description    (string, optional, Description of Network)
  purpose        (string, optional, related to Load Balancing)
  role           (string, optional, related to Load Balancing)
  EOT
  type        = list(map(string))
  default     = []
}

variable "network_routing_mode" {
  type        = string
  default     = "GLOBAL"
  description = "The network routing mode (default \"GLOBAL\")"

  validation {
    condition     = contains(["GLOBAL", "REGIONAL"], var.network_routing_mode)
    error_message = "The network routing mode must either be \"GLOBAL\" or \"REGIONAL\"."
  }
}

variable "network_description" {
  type        = string
  description = "An optional description of this resource (changes will trigger resource destroy/create)"
  default     = ""
}

variable "ips_per_nat" {
  type        = number
  description = "The number of IP addresses to allocate for each regional Cloud NAT (set to 0 to disable NAT)"
  default     = 2
}

variable "shared_vpc_host" {
  type        = bool
  description = "Makes this project a Shared VPC host if 'true' (default 'false')"
  default     = false
}

variable "delete_default_internet_gateway_routes" {
  type        = bool
  description = "If set, ensure that all routes within the network specified whose names begin with 'default-route' and with a next hop of 'default-internet-gateway' are deleted"
  default     = false
}
