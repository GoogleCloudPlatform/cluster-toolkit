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

variable "mtu" {
  type        = number
  description = "The network MTU (default: 8896). Recommended values: 0 (use Compute Engine default), 1460 (default outside HPC environments), 1500 (Internet default), or 8896 (for Jumbo packets). Allowed are all values in the range 1300 to 8896, inclusively."
  default     = 8896
}

variable "subnetworks_template" {
  description = <<-EOT
  Specifications for the subnetworks that will be created within this VPC.
  
  count       (number, required, number of subnets to create, default is 8)
  name_prefix (string, required, subnet name prefix, default is deployment name)
  ip_range    (string, required, range of IPs for all subnets to share (CIDR format), default is 192.168.0.0/16)
  region      (string, optional, region to deploy subnets to, defaults to vars.region)
  EOT
  nullable    = true
  type = object({
    count       = number
    name_prefix = string
    ip_range    = string
    region      = optional(string)
  })
  default = {
    count       = 8
    name_prefix = null
    ip_range    = "192.168.0.0/16"
    region      = null
  }

  validation {
    # If it's NOT a RoCE Metal profile, the template cannot be null
    condition = (
      can(regex("vpc-roce-metal", var.network_profile)) ?
      var.subnetworks_template == null :
      var.subnetworks_template != null
    )
    error_message = "subnetworks_template must be null when using 'vpc-roce-metal' network profile and non-null for all other profiles."
  }

  validation {
    # If template is provided, count must be > 0
    condition     = var.subnetworks_template == null ? true : var.subnetworks_template.count > 0
    error_message = "Number of subnetworks must be greater than 0."
  }

  validation {
    condition     = can(cidrhost(var.subnetworks_template.ip_range, 0))
    error_message = "IP address range must be in CIDR format."
  }
}

variable "network_routing_mode" {
  type        = string
  default     = "REGIONAL"
  description = "The network routing mode (default \"REGIONAL\")"

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

variable "enable_internal_traffic" { # tflint-ignore: terraform_unused_declarations
  description = "DEPRECATED: enable_internal_traffic can not be specified for gpu-rdma-vpc."
  type        = bool
  default     = null
  validation {
    condition     = var.enable_internal_traffic == null
    error_message = "DEPRECATED: enable_internal_traffic can not be specified for gpu-rdma-vpc."
  }
}

variable "firewall_rules" { # tflint-ignore: terraform_unused_declarations
  description = "DEPRECATED: firewall_rules can not be specified for gpu-rdma-vpc."
  type        = any
  default     = null
  validation {
    condition     = var.firewall_rules == null
    error_message = "DEPRECATED: firewall_rules can not be specified for gpu-rdma-vpc."
  }
}

variable "firewall_log_config" { # tflint-ignore: terraform_unused_declarations
  description = "DEPRECATED: firewall_log_config can not be specified for gpu-rdma-vpc."
  type        = string
  default     = null
  validation {
    condition     = var.firewall_log_config == null
    error_message = "DEPRECATED: firewall_log_config can not be specified for gpu-rdma-vpc."
  }
}

variable "network_profile" {
  description = <<-EOT
  A full or partial URL of the network profile to apply to this network.
  This field can be set only at resource creation time. For example, the
  following are valid URLs:
  - https://www.googleapis.com/compute/beta/projects/{projectId}/global/networkProfiles/{network_profile_name}
  - projects/{projectId}/global/networkProfiles/{network_profile_name}}
  EOT
  type        = string
  nullable    = false

  validation {
    condition     = can(coalesce(var.network_profile))
    error_message = "var.network_profile must be specified and not an empty string"
  }
}

variable "nic_type" {
  description = "NIC type for use in modules that use the output"
  type        = string
  nullable    = true
  default     = "MRDMA"

  validation {
    condition     = contains(["MRDMA"], var.nic_type)
    error_message = "The nic_type must be \"MRDMA\"."
  }
}
