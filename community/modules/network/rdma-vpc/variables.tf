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

variable "project_id" {
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "network_name" {
  description = "The name of the network to be created (if unsupplied, will default to \"{deployment_name}-net\")"
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

variable "mtu" {
  type        = number
  description = "The network MTU (default: 8896). Recommended values: 0 (use Compute Engine default), 1460 (default outside HPC environments), 1500 (Internet default), or 8896 (for Jumbo packets). Allowed are all values in the range 1300 to 8896, inclusively."
  default     = 8896
}

variable "subnetworks_template" {
  # TODO: Add validation and improve description
  description = "Rules for creating subnetworks within the VPC"
  type = object({
    count          = number
    name_prefix    = string
    ip_range       = string
    region         = string
    private_access = optional(bool)
  })
  default = {
    count       = 8
    name_prefix = "subnet"
    ip_range    = "192.168.0.0/16"
    region      = null
  }
}

variable "secondary_ranges" {
  type        = map(list(object({ range_name = string, ip_cidr_range = string })))
  description = "Secondary ranges that will be used in some of the subnets. Please see https://goo.gle/hpc-toolkit-vpc-deprecation for migration instructions."
  default     = {}
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

variable "enable_iap_ssh_ingress" {
  type        = bool
  description = "Enable a firewall rule to allow SSH access using IAP tunnels"
  default     = true
}

variable "enable_iap_rdp_ingress" {
  type        = bool
  description = "Enable a firewall rule to allow Windows Remote Desktop Protocol access using IAP tunnels"
  default     = false
}

variable "enable_iap_winrm_ingress" {
  type        = bool
  description = "Enable a firewall rule to allow Windows Remote Management (WinRM) access using IAP tunnels"
  default     = false
}

variable "enable_internal_traffic" {
  type        = bool
  description = "Enable a firewall rule to allow all internal TCP, UDP, and ICMP traffic within the network"
  default     = true
}

variable "extra_iap_ports" {
  type        = list(string)
  description = "A list of TCP ports for which to create firewall rules that enable IAP for TCP forwarding (use dedicated enable_iap variables for standard ports)"
  default     = []
}

variable "allowed_ssh_ip_ranges" {
  type        = list(string)
  description = "A list of CIDR IP ranges from which to allow ssh access"
  default     = []

  validation {
    condition     = alltrue([for r in var.allowed_ssh_ip_ranges : can(cidrhost(r, 32))])
    error_message = "Each element of var.allowed_ssh_ip_ranges must be a valid CIDR-formatted IPv4 range."
  }
}

variable "firewall_rules" {
  type        = any
  description = "List of firewall rules"
  default     = []
}

variable "firewall_log_config" {
  type        = string
  description = "Firewall log configuration for Toolkit firewall rules (var.enable_iap_ssh_ingress and others)"
  default     = "DISABLE_LOGGING"
  nullable    = false

  validation {
    condition = contains([
      "INCLUDE_ALL_METADATA",
      "EXCLUDE_ALL_METADATA",
      "DISABLE_LOGGING",
    ], var.firewall_log_config)
    error_message = "var.firewall_log_config must be set to \"DISABLE_LOGGING\", or enable logging with \"INCLUDE_ALL_METADATA\" or \"EXCLUDE_ALL_METADATA\""
  }
}

variable "network_profile" {
  # TODO Update this description
  description = "Profile name for VPC configuration"
  type        = string
  default     = null
}

variable "nic_type" {
  description = "NIC type for use in modules that use the output"
  type        = string
  default     = null
}
