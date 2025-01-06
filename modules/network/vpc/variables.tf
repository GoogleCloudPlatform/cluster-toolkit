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

variable "labels" {
  description = "Labels to add to network resources that support labels. Key-value pairs of strings."
  type        = map(string)
  default     = {}
  nullable    = false
}

variable "network_name" {
  description = "The name of the network to be created (if unsupplied, will default to \"{deployment_name}-net\")"
  type        = string
  default     = null
}

variable "subnetwork_name" {
  description = "The name of the network to be created (if unsupplied, will default to \"{deployment_name}-primary-subnet\")"
  type        = string
  default     = null
}

# tflint-ignore: terraform_unused_declarations
variable "subnetwork_size" {
  description = "DEPRECATED: please see https://goo.gle/hpc-toolkit-vpc-deprecation for migration instructions"
  type        = number
  default     = null
  validation {
    condition     = var.subnetwork_size == null
    error_message = "subnetwork_size is deprecated. Please see https://goo.gle/hpc-toolkit-vpc-deprecation for migration instructions."
  }
}

variable "default_primary_subnetwork_size" {
  description = "The size, in CIDR bits, of the default primary subnetwork unless explicitly defined in var.subnetworks"
  type        = number
  default     = 15
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

variable "subnetworks" {
  description = <<-EOT
  List of subnetworks to create within the VPC. If left empty, it will be
  replaced by a single, default subnetwork constructed from other parameters
  (e.g. var.region). In all cases, the first subnetwork in the list is identified
  by outputs as a "primary" subnetwork.

  subnet_name           (string, required, name of subnet)
  subnet_region         (string, required, region of subnet)
  subnet_ip             (string, mutually exclusive with new_bits, CIDR-formatted IP range for subnetwork)
  new_bits              (number, mutually exclusive with subnet_ip, CIDR bits used to calculate subnetwork range)
  subnet_private_access (bool, optional, Enable Private Access on subnetwork)
  subnet_flow_logs      (map(string), optional, Configure Flow Logs see terraform-google-network module)
  description           (string, optional, Description of Network)
  purpose               (string, optional, related to Load Balancing)
  role                  (string, optional, related to Load Balancing)
  EOT
  type        = list(map(string))
  default     = []
  validation {
    condition = alltrue([
      for s in var.subnetworks : can(s["subnet_name"])
    ])
    error_message = "All subnetworks must define \"subnet_name\"."
  }
  validation {
    condition = alltrue([
      for s in var.subnetworks : can(s["subnet_region"])
    ])
    error_message = "All subnetworks must define \"subnet_region\"."
  }
  validation {
    condition = alltrue([
      for s in var.subnetworks : can(s["subnet_ip"]) != can(s["new_bits"])
    ])
    error_message = "All subnetworks must define exactly one of \"subnet_ip\" or \"new_bits\"."
  }
  validation {
    condition     = alltrue([for s in var.subnetworks : can(s["subnet_ip"])]) || alltrue([for s in var.subnetworks : can(s["new_bits"])])
    error_message = "All subnetworks must make same choice of \"subnet_ip\" or \"new_bits\"."
  }
}

# tflint-ignore: terraform_unused_declarations
variable "primary_subnetwork" {
  description = "DEPRECATED: please see https://goo.gle/hpc-toolkit-vpc-deprecation for migration instructions"
  type        = map(string)
  default     = null
  validation {
    condition     = var.primary_subnetwork == null
    error_message = "primary_subnetwork is deprecated. Please see https://goo.gle/hpc-toolkit-vpc-deprecation for migration instructions."
  }
}

# tflint-ignore: terraform_unused_declarations
variable "additional_subnetworks" {
  description = "DEPRECATED: please see https://goo.gle/hpc-toolkit-vpc-deprecation for migration instructions"
  type        = list(map(string))
  default     = null
  validation {
    condition     = var.additional_subnetworks == null
    error_message = "additional_subnetworks is deprecated. Please see https://goo.gle/hpc-toolkit-vpc-deprecation for migration instructions."
  }
}

variable "secondary_ranges" {
  type        = map(list(object({ range_name = string, ip_cidr_range = string })))
  description = <<-EOT
  "Secondary ranges associated with the subnets.
  This will be deprecated in favour of secondary_ranges_list at a later date.
  Please migrate to using the same."
  EOT
  default     = {}
}

variable "secondary_ranges_list" {
  type = list(object({
    subnetwork_name = string,
    ranges = list(object({
      range_name    = string,
      ip_cidr_range = string
    }))
  }))
  description = "List of secondary ranges associated with the subnets."
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

variable "enable_cloud_router" {
  type        = bool
  description = "Enable the creation of a Cloud Router for your VPC. For more information on Cloud Routers see https://cloud.google.com/network-connectivity/docs/router/concepts/overview"
  default     = true
}

variable "enable_cloud_nat" {
  type        = bool
  description = "Enable the creation of Cloud NATs."
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

resource "terraform_data" "secondary_ranges_validation" {
  lifecycle {
    precondition {
      condition     = length(var.secondary_ranges) == 0 || length(var.secondary_ranges_list) == 0
      error_message = "Only one of var.secondary_ranges or var.secondary_ranges_list should be specified"
    }
  }
}

variable "network_profile" {
  type        = string
  description = <<-EOT
  A full or partial URL of the network profile to apply to this network.
  This field can be set only at resource creation time. For example, the
  following are valid URLs:
  - https://www.googleapis.com/compute/beta/projects/{projectId}/global/networkProfiles/{network_profile_name}
  - projects/{projectId}/global/networkProfiles/{network_profile_name}}
  EOT
  default     = null
}
