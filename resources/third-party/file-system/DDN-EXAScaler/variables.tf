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


# EXAScaler filesystem name
# only alphanumeric characters are allowed,
# and the value must be 1-8 characters long
variable "fsname" {
  description = "EXAScaler filesystem name, only alphanumeric characters are allowed, and the value must be 1-8 characters long"
  type        = string
  default     = "exacloud"
}

# Project ID
# https://cloud.google.com/resource-manager/docs/creating-managing-projects
variable "project_id" {
  description = "Compute Platform project that will host the EXAScaler filesystem"
  type        = string
}

# Zone name to manage resources
# https://cloud.google.com/compute/docs/regions-zones
variable "zone" {
  description = "Compute Platform zone where the servers will be located"
  type        = string
}

# Service account name used by deploy application
# https://cloud.google.com/iam/docs/service-accounts
# new: create a new custom service account, or use an existing one: true or false
# name: existing service account name, will be using if new is false
variable "service_account" {
  description = "Service account name used by deploy application"
  type = object({
    new  = bool
    name = string
  })
  default = {
    new  = false
    name = "default"
  }
}

# Waiter to check progress and result for deployment.
# To use Google Deployment Manager:
# waiter = "deploymentmanager"
# To use generic Google Cloud SDK command line:
# waiter = "sdk"
# If you don’t want to wait until the deployment is complete:
# waiter = null
# https://cloud.google.com/deployment-manager/runtime-configurator/creating-a-waiter
variable "waiter" {
  description = "Waiter to check progress and result for deployment."
  type        = string
  default     = null
}

# Security options
# admin: optional user name for remote SSH access
# Set admin = null to disable creation admin user
# public_key: path to the SSH public key on the local host
# Set public_key = null to disable creation admin user
# block_project_keys: true or false
# Block project-wide public SSH keys if you want to restrict
# deployment to only user with deployment-level public SSH key.
# https://cloud.google.com/compute/docs/instances/adding-removing-ssh-keys
# enable_local: true or false, enable or disable firewall rules for local access
# enable_ssh: true or false, enable or disable remote SSH access
# ssh_source_ranges: source IP ranges for remote SSH access in CIDR notation
# enable_http: true or false, enable or disable remote HTTP access
# http_source_ranges: source IP ranges for remote HTTP access in CIDR notation
variable "security" {
  description = "Security options"
  type = object({
    admin              = string
    public_key         = string
    block_project_keys = bool
    enable_local       = bool
    enable_ssh         = bool
    enable_http        = bool
    ssh_source_ranges  = list(string)
    http_source_ranges = list(string)
  })

  default = {
    admin              = "stack"
    public_key         = "~/.ssh/id_rsa.pub"
    block_project_keys = false
    enable_local       = false
    enable_ssh         = false
    enable_http        = false
    ssh_source_ranges = [
      "0.0.0.0/0"
    ]
    http_source_ranges = [
      "0.0.0.0/0"
    ]
  }
}

variable "network_self_link" {
  description = "The self-link of the VPC network to where the system is connected."
  type        = string
  default     = null
}

# Network properties
# https://cloud.google.com/vpc/docs/vpc
# routing: network-wide routing mode: REGIONAL or GLOBAL
# tier: networking tier for VM interfaces: STANDARD or PREMIUM
# id: existing network id, will be using if new is false
# auto: create subnets in each region automatically: false or true
# mtu: maximum transmission unit in bytes: 1460 - 1500
# new: create a new network, or use an existing one: true or false
# nat: allow instances without external IP to communicate with the outside world: true or false
variable "network" {
  description = "Network options"
  type = object({
    routing = string
    tier    = string
    id      = string
    auto    = bool
    mtu     = number
    new     = bool
    nat     = bool
  })

  default = {
    routing = "REGIONAL"
    tier    = "STANDARD"
    id      = "projects/project-name/global/networks/network-name"
    auto    = false
    mtu     = 1500
    new     = false
    nat     = false
  }
}

variable "subnetwork_self_link" {
  description = "The self-link of the VPC subnetwork to where the system is connected."
  type        = string
  default     = null
}

variable "subnetwork_address" {
  description = "The IP range of internal addresses for the subnetwork"
  type        = string
  default     = null
}

# Subnetwork properties
# https://cloud.google.com/vpc/docs/vpc
# address: IP range of internal addresses for a new subnetwork
# private: when enabled VMs in this subnetwork without external
# IP addresses can access Google APIs and services by using
# Private Google Access: true or false
# https://cloud.google.com/vpc/docs/private-access-options
# id: existing subnetwork id, will be using if new is false
# new: create a new subnetwork, or use an existing one: true or false
variable "subnetwork" {
  description = "Subnetwork properties. Ignored if subnetwork_self_link is supplied."
  type = object({
    address = string
    private = bool
    id      = string
    new     = bool
  })
  default = {
    address = "10.0.0.0/16"
    private = true
    id      = "projects/project-name/regions/region-name/subnetworks/subnetwork-name"
    new     = false
  }
}
# Boot disk properties
# disk_type: pd-standard, pd-ssd or pd-balanced
# auto_delete: true or false
# whether the disk will be auto-deleted when the instance is deleted
variable "boot" {
  description = "Boot disk properties"
  type = object({
    disk_type   = string
    auto_delete = bool
  })
  default = {
    disk_type   = "pd-standard"
    auto_delete = true
  }
}

# Source image properties
# project: project name
# name: image name
variable "image" {
  description = "Source image properties"
  type = object({
    project = string
    name    = string
  })
  default = {
    project = "ddn-public"
    name    = "exascaler-cloud-v522-centos7"
  }
}

# Management server properties
# https://cloud.google.com/compute/docs/machine-types
# https://cloud.google.com/compute/docs/instances/specify-min-cpu-platform
# https://cloud.google.com/compute/docs/networking/using-gvnic
# nic_type: GVNIC or VIRTIO_NET
# public_ip: true or false
# node_count: number of instances
variable "mgs" {
  description = "Management server properties"
  type = object({
    node_type  = string
    node_cpu   = string
    nic_type   = string
    node_count = number
    public_ip  = bool
  })
  default = {
    node_type  = "n2-standard-32"
    node_cpu   = "Intel Cascade Lake"
    nic_type   = "GVNIC"
    public_ip  = true
    node_count = 1
  }
}

# Management target properties
# https://cloud.google.com/compute/docs/disks
# disk_bus: SCSI or NVME (NVME is for scratch disks only)
# disk_type: pd-standard, pd-ssd, pd-balanced or scratch
# disk_size: target size in in GB
# scratch disk size must be exactly 375
# disk_count: number of targets
variable "mgt" {
  description = "Management target properties"
  type = object({
    disk_bus   = string
    disk_type  = string
    disk_size  = number
    disk_count = number
  })
  default = {
    disk_bus   = "SCSI"
    disk_type  = "pd-standard"
    disk_size  = 128
    disk_count = 1
  }
}


# Monitoring target properties
# https://cloud.google.com/compute/docs/disks
# disk_bus: SCSI or NVME (NVME is for scratch disks only)
# disk_type: pd-standard, pd-ssd, pd-balanced or scratch
# disk_size: target size in in GB
# scratch disk size must be exactly 375
# disk_count: number of targets
variable "mnt" {
  description = "Monitoring target properties"
  type = object({
    disk_bus   = string
    disk_type  = string
    disk_size  = number
    disk_count = number
  })
  default = {
    disk_bus   = "SCSI"
    disk_type  = "pd-standard"
    disk_size  = 128
    disk_count = 1
  }
}
# Metadata server properties
# https://cloud.google.com/compute/docs/machine-types
# https://cloud.google.com/compute/docs/instances/specify-min-cpu-platform
# https://cloud.google.com/compute/docs/networking/using-gvnic
# nic_type: GVNIC or VIRTIO_NET
# public_ip: true or false
# node_count: number of instances
variable "mds" {
  description = "Metadata server properties"
  type = object({
    node_type  = string
    node_cpu   = string
    nic_type   = string
    node_count = number
    public_ip  = bool
  })
  default = {
    node_type  = "n2-standard-32"
    node_cpu   = "Intel Cascade Lake"
    nic_type   = "GVNIC"
    public_ip  = true
    node_count = 1
  }
}

# Metadata target properties
# https://cloud.google.com/compute/docs/disks
# disk_bus: SCSI or NVME (NVME is for scratch disks only)
# disk_type: pd-standard, pd-ssd, pd-balanced or scratch
# disk_size: target size in in GB
# scratch disk size must be exactly 375
# disk_count: number of targets
variable "mdt" {
  description = "Metadata target properties"
  type = object({
    disk_bus   = string
    disk_type  = string
    disk_size  = number
    disk_count = number
  })
  default = {
    disk_bus   = "SCSI"
    disk_type  = "pd-ssd"
    disk_size  = 3500
    disk_count = 1
  }
}

# Object Storage server properties
# https://cloud.google.com/compute/docs/machine-types
# https://cloud.google.com/compute/docs/instances/specify-min-cpu-platform
# https://cloud.google.com/compute/docs/networking/using-gvnic
# nic_type: GVNIC or VIRTIO_NET
# public_ip: true or false
variable "oss" {
  description = "Object Storage server properties"
  type = object({
    node_type  = string
    node_cpu   = string
    nic_type   = string
    node_count = number
    public_ip  = bool
  })
  default = {
    node_type  = "n2-standard-16"
    node_cpu   = "Intel Cascade Lake"
    nic_type   = "GVNIC"
    public_ip  = true
    node_count = 3
  }
}

# Object Storage target properties
# https://cloud.google.com/compute/docs/disks
# disk_bus: SCSI or NVME (NVME is for scratch disks only)
# disk_type: pd-standard, pd-ssd, pd-balanced or scratch
# disk_size: target size in in GB
# scratch disk size must be exactly 375
# disk_count: number of targets
variable "ost" {
  description = "Object Storage target properties"
  type = object({
    disk_bus   = string
    disk_type  = string
    disk_size  = number
    disk_count = number
  })
  default = {
    disk_bus   = "SCSI"
    disk_type  = "pd-ssd"
    disk_size  = 3500
    disk_count = 1
  }
}

# Compute client properties
# https://cloud.google.com/compute/docs/machine-types
# https://cloud.google.com/compute/docs/instances/specify-min-cpu-platform
# https://cloud.google.com/compute/docs/networking/using-gvnic
# nic_type: GVNIC or VIRTIO_NET
# public_ip: true or false
# node_count: number of instances
variable "cls" {
  description = "Compute client properties"
  type = object({
    node_type  = string
    node_cpu   = string
    nic_type   = string
    node_count = number
    public_ip  = bool
  })
  default = {
    node_type  = "n2-standard-2"
    node_cpu   = "Intel Cascade Lake"
    nic_type   = "GVNIC"
    public_ip  = true
    node_count = 0
  }
}
# Compute client target properties
# https://cloud.google.com/compute/docs/disks
# disk_bus: SCSI or NVME (NVME is for scratch disks only)
# disk_type: pd-standard, pd-ssd, pd-balanced or scratch
# disk_size: target size in in GB
# scratch disk size must be exactly 375
# disk_count: number of targets, 0 to disable
variable "clt" {
  description = "Compute client target properties"
  type = object({
    disk_bus   = string
    disk_type  = string
    disk_size  = number
    disk_count = number
  })
  default = {
    disk_bus   = "SCSI"
    disk_type  = "pd-standard"
    disk_size  = 256
    disk_count = 0
  }
}
variable "local_mount" {
  description = "Mountpoint (at the client instances) for this EXAScaler system"
  type        = string
  default     = "/shared"
}
