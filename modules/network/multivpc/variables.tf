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
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "deployment_name" {
  description = "The name of the current deployment"
  type        = string
}

variable "region" {
  description = "The default region for Cloud resources"
  type        = string
}

variable "network_count" {
  description = "The number of vpc nettworks to create"
  type        = number
  default     = 4

  validation {
    condition     = var.network_count > 1
    error_message = "The minimum VPCs able to be created by this module is 2. Use the standard Toolkit module at modules/network/vpc for count = 1"
  }
  validation {
    condition     = var.network_count < 9
    error_message = "The maximum VPCs able to be created by this module is 8"
  }
}

variable "super_global_ip_address_range" {
  description = "IP address range (CIDR) that will span entire set of VPC networks"
  type        = string
  default     = "172.16.0.0"

  validation {
    condition     = can(cidrhost(var.super_global_ip_address_range, 0))
    error_message = "var.super_global_ip_address_range must be an IPv4 CIDR range (e.g. \"172.16.0.0/9\")."
  }
}

variable "network_cidr_prefix" {
  description = "The size, in CIDR prefix notation, for each network (e.g. 24 for 172.16.0.0/24); changing this will destroy every network."
  type        = number
  default     = 16
}

variable "mtu" {
  type        = number
  description = "The network MTU (default: 8896). Recommended values: 0 (use Compute Engine default), 1460 (default outside HPC environments), 1500 (Internet default), or 8896 (for Jumbo packets). Allowed are all values in the range 1300 to 8896, inclusively."
  default     = 8896
}

variable "secondary_ranges" {
  type        = map(list(object({ range_name = string, ip_cidr_range = string })))
  description = "Secondary ranges that will be used in some of the subnets. Please see https://goo.gle/hpc-toolkit-vpc-deprecation for migration instructions."
  default     = {}
}

variable "network_routing_mode" {
  type        = string
  default     = "REGIONAL"
  description = "The network dynamic routing mode"

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
