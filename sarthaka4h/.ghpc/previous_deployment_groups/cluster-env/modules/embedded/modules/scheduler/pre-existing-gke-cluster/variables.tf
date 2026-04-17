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
  description = "Project that hosts the existing cluster"
  type        = string
}

variable "cluster_name" {
  description = "Name of the existing cluster"
  type        = string
}

variable "region" {
  description = "Region in which to search for the cluster"
  type        = string
}

variable "additional_networks" {
  description = "Additional network interface details for GKE, if any. Providing additional networks creates relevat network objects on the cluster."
  default     = []
  type = list(object({
    network            = string
    subnetwork         = string
    subnetwork_project = string
    network_ip         = string
    nic_type           = string
    stack_type         = string
    queue_count        = number
    access_config = list(object({
      nat_ip       = string
      network_tier = string
    }))
    ipv6_access_config = list(object({
      network_tier = string
    }))
    alias_ip_range = list(object({
      ip_cidr_range         = string
      subnetwork_range_name = string
    }))
  }))
}

variable "rdma_subnetwork_name_prefix" {
  description = "Prefix of the RDMA subnetwork names"
  default     = null
  type        = string
}
