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

# User for remote SSH access
# username: remote user name
# ssh_public_key: path local SSH public key
# https://cloud.google.com/compute/docs/instances/adding-removing-ssh-keys
variable "admin" {
  description = "User for remote SSH access"
  type = object({
    username       = string
    ssh_public_key = string
  })
  default = {
    ssh_public_key = "~/.ssh/id_rsa.pub"
    username       = "admin"
  }
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

# Enable/disable remote SSH access: true or false
# Source IP for remote SSH access: valid CIDR range of the form x.x.x.x/x
# Enable/disable remote HTTP console: true or false
# Source IP for remote HTTP access valid CIDR range of the form x.x.x.x/x
variable "security" {
  description = "Enables/disables SSH and HTTP access"
  type = object({
    enable_ssh        = bool
    ssh_source_range  = string
    enable_http       = bool
    http_source_range = string
  })

  default = {
    enable_http       = false
    enable_ssh        = false
    http_source_range = "0.0.0.0/0"
    ssh_source_range  = "0.0.0.0/0"
  }
}

variable "network_name" {
  description = "The name of the VPC network to where the system is connected."
  type        = string
  default     = null
}

# Network properties
# https://cloud.google.com/vpc/docs/vpc
# routing: network-wide routing mode: REGIONAL or GLOBAL
# tier: networking tier for VM interfaces: STANDARD or PREMIUM
# name: existing network name, will be using if new is false
# auto: create subnets in each region automatically: false or true
# mtu: maximum transmission unit in bytes: 1460 - 1500
# new: create a new network, or use an existing one: true or false
# nat: allow instances without external IP to communicate with the outside world: true or false
variable "network_properties" {
  description = "Network properties. Ignored if network_name is supplied."
  type = object({
    routing = string
    tier    = string
    name    = string
    auto    = bool
    mtu     = number
    new     = bool
    nat     = bool
  })
  default = {
    routing = "REGIONAL"
    tier    = "STANDARD"
    name    = "default"
    auto    = false
    mtu     = 1500
    new     = false
    nat     = false
  }
}

variable "subnetwork_name" {
  description = "The name of the VPC subnetwork to where the system is connected."
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
# name: existing subnetwork name, will be using if new is false
# new: create a new subnetwork, or use an existing one: true or false
variable "subnetwork_properties" {
  description = "Subnetwork properties. Ignored if subnetwork_name is supplied."
  type = object({
    address = string
    private = bool
    name    = string
    new     = bool
  })
  default = {
    address = "10.0.0.0/16"
    private = true
    name    = "default"
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
    node_type  = "n2-standard-2"
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
