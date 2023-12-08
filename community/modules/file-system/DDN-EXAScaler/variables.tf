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


# EXAScaler filesystem name
# only alphanumeric characters are allowed,
# and the value must be 1-8 characters long
variable "fsname" {
  description = "EXAScaler filesystem name, only alphanumeric characters are allowed, and the value must be 1-8 characters long"
  type        = string
  default     = "exacloud"
}

# Project ID to manage resources
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
# new: create a new custom service account or use an existing one: true or false
# email: existing service account email address, will be using if new is false
# set email = null to use the default compute service account
variable "service_account" {
  description = "Service account name used by deploy application"
  type = object({
    new   = bool
    email = string
  })
  default = {
    new   = false
    email = null
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
# enable_os_login: true or false
# Enable or disable OS Login feature.
# Please note, enabling this option disables other security options:
# admin, public_key and block_project_keys.
# https://cloud.google.com/compute/docs/instances/managing-instance-access#enable_oslogin
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
    enable_os_login    = bool
    enable_local       = bool
    enable_ssh         = bool
    enable_http        = bool
    ssh_source_ranges  = list(string)
    http_source_ranges = list(string)
  })

  default = {
    admin              = "stack"
    public_key         = null
    block_project_keys = false
    enable_os_login    = true
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
  description = "The self-link of the VPC network to where the system is connected.  Ignored if 'network_properties' is provided. 'network_self_link' or 'network_properties' must be provided."
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
# new: create a new network or use an existing one: true or false
# nat: allow instances without external IP to communicate with the outside world: true or false
variable "network_properties" {
  description = "Network options. 'network_self_link' or 'network_properties' must be provided."
  type = object({
    routing = string
    tier    = string
    id      = string
    auto    = bool
    mtu     = number
    new     = bool
    nat     = bool
  })

  default = null
}

variable "subnetwork_self_link" {
  description = "The self-link of the VPC subnetwork to where the system is connected. Ignored if 'subnetwork_properties' is provided. 'subnetwork_self_link' or 'subnetwork_properties' must be provided."
  type        = string
  default     = null
}

variable "subnetwork_address" {
  description = "The IP range of internal addresses for the subnetwork. Ignored if 'subnetwork_properties' is provided."
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
# new: create a new subnetwork or use an existing one: true or false
variable "subnetwork_properties" {
  description = "Subnetwork properties. 'subnetwork_self_link' or 'subnetwork_properties' must be provided."
  type = object({
    address = string
    private = bool
    id      = string
    new     = bool
  })
  default = null
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
    script_url  = string
  })
  default = {
    disk_type   = "pd-standard"
    auto_delete = true
    script_url  = null
  }
}

# Source image properties
# project: project name
# family: image family name
# name: !!DEPRECATED!! - image name
variable "image" {
  description = "DEPRECATED: Source image properties"
  type        = any
  # Ommiting type checking so validation can provide more useful error message
  # type = object({
  #   project = string
  #   family  = string
  # })
  default = null

  validation {
    condition     = var.image == null
    error_message = "The 'var.image' setting is deprecated, please use 'var.instance_image' with the fields 'project' and 'family' or 'name'."
  }
}

variable "instance_image" {
  description = <<-EOD
    Source image properties

    Expected Fields:
    name: Unavailable with this module.
    family: The image family to use.
    project: The project where the image is hosted.
    EOD
  type        = map(string)
  default = {
    project = "ddn-public"
    family  = "exascaler-cloud-6-2-rocky-linux-8-optimized-gcp"
  }

  validation {
    condition     = !can(coalesce(var.instance_image.name))
    error_message = "In var.instance_image, the \"name\" field is not used, please use the \"family\" setting."
  }

  validation {
    condition     = can(coalesce(var.instance_image.project))
    error_message = "In var.instance_image, the \"project\" field must be a string set to the Cloud project ID."
  }

  validation {
    condition     = can(coalesce(var.instance_image.family))
    error_message = "In var.instance_image, the \"family\" field must be a string set to the image family."
  }
}

# Management server properties
# node_type: type of management server
# https://cloud.google.com/compute/docs/machine-types
# node_cpu: CPU family
# https://cloud.google.com/compute/docs/instances/specify-min-cpu-platform
# nic_type: type of network connectivity, GVNIC or VIRTIO_NET
# https://cloud.google.com/compute/docs/networking/using-gvnic
# public_ip: assign an external IP address, true or false
# node_count: number of management servers
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
# disk_bus: type of management target interface, SCSI or NVME (NVME is for scratch disks only)
# disk_type: type of management target, pd-standard, pd-ssd, pd-balanced or scratch
# disk_size: size of management target in GB (scratch disk size must be exactly 375)
# disk_count: number of management targets
# disk_raid: create striped management target, true or false
variable "mgt" {
  description = "Management target properties"
  type = object({
    disk_bus   = string
    disk_type  = string
    disk_size  = number
    disk_count = number
    disk_raid  = bool
  })
  default = {
    disk_bus   = "SCSI"
    disk_type  = "pd-standard"
    disk_size  = 128
    disk_count = 1
    disk_raid  = false
  }
}


# Monitoring target properties
# https://cloud.google.com/compute/docs/disks
# disk_bus: type of monitoring target interface, SCSI or NVME (NVME is for scratch disks only)
# disk_type: type of monitoring target, pd-standard, pd-ssd, pd-balanced or scratch
# disk_size: size of monitoring target in GB (scratch disk size must be exactly 375)
# disk_count: number of monitoring targets
# disk_raid: create striped monitoring target, true or false
variable "mnt" {
  description = "Monitoring target properties"
  type = object({
    disk_bus   = string
    disk_type  = string
    disk_size  = number
    disk_count = number
    disk_raid  = bool
  })
  default = {
    disk_bus   = "SCSI"
    disk_type  = "pd-standard"
    disk_size  = 128
    disk_count = 1
    disk_raid  = false
  }
}

# Metadata server properties
# node_type: type of metadata server
# https://cloud.google.com/compute/docs/machine-types
# node_cpu: CPU family
# https://cloud.google.com/compute/docs/instances/specify-min-cpu-platform
# nic_type: type of network connectivity, GVNIC or VIRTIO_NET
# https://cloud.google.com/compute/docs/networking/using-gvnic
# public_ip: assign an external IP address, true or false
# node_count: number of metadata servers
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
# disk_bus: type of metadata target interface, SCSI or NVME (NVME is for scratch disks only)
# disk_type: type of metadata target, pd-standard, pd-ssd, pd-balanced or scratch
# disk_size: size of metadata target in GB (scratch disk size must be exactly 375)
# disk_count: number of metadata targets
# disk_raid: create striped metadata target, true or false
variable "mdt" {
  description = "Metadata target properties"
  type = object({
    disk_bus   = string
    disk_type  = string
    disk_size  = number
    disk_count = number
    disk_raid  = bool
  })
  default = {
    disk_bus   = "SCSI"
    disk_type  = "pd-ssd"
    disk_size  = 3500
    disk_count = 1
    disk_raid  = false
  }
}

# Object Storage server properties
# node_type: type of storage server
# https://cloud.google.com/compute/docs/machine-types
# node_cpu: CPU family
# https://cloud.google.com/compute/docs/instances/specify-min-cpu-platform
# nic_type: type of network connectivity, GVNIC or VIRTIO_NET
# https://cloud.google.com/compute/docs/networking/using-gvnic
# public_ip: assign an external IP address, true or false
# node_count: number of storage servers
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
# disk_bus: type of storage target interface, SCSI or NVME (NVME is for scratch disks only)
# disk_type: type of storage target, pd-standard, pd-ssd, pd-balanced or scratch
# disk_size: size of storage target in GB (scratch disk size must be exactly 375)
# disk_count: number of storage targets
# disk_raid: create striped storage target, true or false
variable "ost" {
  description = "Object Storage target properties"
  type = object({
    disk_bus   = string
    disk_type  = string
    disk_size  = number
    disk_count = number
    disk_raid  = bool
  })
  default = {
    disk_bus   = "SCSI"
    disk_type  = "pd-ssd"
    disk_size  = 3500
    disk_count = 1
    disk_raid  = false
  }
}

# Compute client properties
# node_type: type of compute client
# https://cloud.google.com/compute/docs/machine-types
# node_cpu: CPU family
# https://cloud.google.com/compute/docs/instances/specify-min-cpu-platform
# nic_type: type of network connectivity, GVNIC or VIRTIO_NET
# https://cloud.google.com/compute/docs/networking/using-gvnic
# public_ip: assign an external IP address, true or false
# node_count: number of compute clients
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
# disk_bus: type of compute target interface, SCSI or NVME (NVME is for scratch disks only)
# disk_type: type of compute target, pd-standard, pd-ssd, pd-balanced or scratch
# disk_size: size of compute target in GB (scratch disk size must be exactly 375)
# disk_count: number of compute targets
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

variable "prefix" {
  description = "EXAScaler Cloud deployment prefix (`null` defaults to 'exascaler-cloud')"
  type        = string
  default     = null
}

variable "labels" {
  description = "Labels to add to EXAScaler Cloud deployment. Key-value pairs."
  type        = map(string)
  default     = {}
}
