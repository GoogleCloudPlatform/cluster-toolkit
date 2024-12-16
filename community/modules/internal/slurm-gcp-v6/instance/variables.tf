/**
 * Copyright (C) SchedMD LLC.
 * Copyright 2018 Google LLC
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
  type        = string
  description = "The GCP project ID"
  default     = null
}

variable "network" {
  description = "Network to deploy to. Only one of network or subnetwork should be specified."
  type        = string
  default     = ""
}

variable "subnetwork" {
  description = "Subnet to deploy to. Only one of network or subnetwork should be specified."
  type        = string
  default     = ""
}

variable "subnetwork_project" {
  description = "The project that subnetwork belongs to"
  type        = string
  default     = null
}

variable "hostname" {
  description = "Hostname of instances"
  type        = string
}

variable "additional_networks" {
  description = "Additional network interface details for GCE, if any."
  default     = []
  type = list(object({
    access_config = optional(list(object({
      nat_ip       = string
      network_tier = string
    })), [])
    alias_ip_range = optional(list(object({
      ip_cidr_range         = string
      subnetwork_range_name = string
    })), [])
    ipv6_access_config = optional(list(object({
      network_tier = string
    })), [])
    network            = optional(string)
    network_ip         = optional(string, "")
    nic_type           = optional(string)
    queue_count        = optional(number)
    stack_type         = optional(string)
    subnetwork         = optional(string)
    subnetwork_project = optional(string)
  }))
  nullable = false
}

variable "static_ips" {
  description = "List of static IPs for VM instances"
  type        = list(string)
  default     = []
}

variable "access_config" {
  description = "Access configurations, i.e. IPs via which the VM instance can be accessed via the Internet."
  type = list(object({
    nat_ip       = string
    network_tier = string
  }))
  default = []
}

variable "num_instances" {
  description = "Number of instances to create. This value is ignored if static_ips is provided."
  type        = number
  default     = 1
}

variable "instance_template" {
  description = "Instance template self_link used to create compute instances"
  type        = string
}

variable "region" {
  description = "Region where the instances should be created."
  type        = string
  default     = null
}

variable "zone" {
  description = "Zone where the instances should be created. If not specified, instances will be spread across available zones in the region."
  type        = string
  default     = null
}

#########
# SLURM #
#########

variable "replace_trigger" {
  description = "Trigger value to replace the instances."
  type        = string
  default     = ""
}
