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
  type        = string
  description = "Project ID to create resources in."
}

variable "deployment_name" {
  description = "Name of the deployment"
  type        = string
}

variable "labels" {
  type        = map(string)
  description = "Labels, provided as a map"
  default     = {}
}

#########
# SLURM #
#########

variable "disable_smt" {
  type        = bool
  description = "Disables Simultaneous Multi-Threading (SMT) on instance."
  default     = false
}

variable "slurm_cluster_name" {
  type        = string
  description = "Cluster name, used for resource naming."

  validation {
    condition     = can(regex("(^[a-z][a-z0-9]*$)", var.slurm_cluster_name))
    error_message = "Variable 'slurm_cluster_name' must be a match of regex '(^[a-z][a-z0-9]*$)'."
  }
}

variable "controller_instance_id" {
  description = "The controller instance template"
  type        = string
}

variable "controller_instance" {
  description = "The controller instance template"
  type        = any
}

###########
# NETWORK #
###########

variable "can_ip_forward" {
  type        = bool
  description = "Enable IP forwarding, for NAT instances for example."
  default     = false
}

variable "network_self_link" {
  type        = string
  description = "Network to deploy to. Only one of network or subnetwork should be specified."
  default     = ""
}

variable "subnetwork_self_link" {
  type        = string
  description = "Subnet to deploy to. Only one of network or subnetwork should be specified."
  default     = ""
}

variable "subnetwork_project" {
  type        = string
  description = "The project that subnetwork belongs to."
  default     = ""
}

variable "region" {
  type        = string
  description = <<-EOD
  Region where the instances should be created.
  Note: region will be ignored if it can be extracted from subnetwork.
  EOD
  default     = null
}

variable "network_ip" {
  type        = string
  description = "Private IP address to assign to the instance if desired."
  default     = ""
}

############
# INSTANCE #
############

variable "static_ips" {
  type        = list(string)
  description = "List of static IPs for VM instances."
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

variable "zone" {
  type        = string
  description = <<EOD
Zone where the instances should be created. If not specified, instances will be
spread across available zones in the region.
EOD
  default     = null
}

variable "metadata" {
  type        = map(string)
  description = "Metadata, provided as a map"
  default     = {}
}

variable "tags" {
  type        = list(string)
  description = "Network tag list."
  default     = []
}

variable "machine_type" {
  type        = string
  description = "Machine type to create."
  default     = "n2-standard-2"
}

variable "min_cpu_platform" {
  type        = string
  description = <<EOD
Specifies a minimum CPU platform. Applicable values are the friendly names of
CPU platforms, such as Intel Haswell or Intel Skylake. See the complete list:
https://cloud.google.com/compute/docs/instances/specify-min-cpu-platform
EOD
  default     = null
}

variable "gpu" {
  type = object({
    type  = string
    count = number
  })
  description = <<EOD
GPU information. Type and count of GPU to attach to the instance template. See
https://cloud.google.com/compute/docs/gpus more details.
* type : the GPU type
* count : number of GPUs
EOD
  default     = null
}

variable "service_account" {
  type = object({
    email  = string
    scopes = set(string)
  })
  description = <<EOD
Service account to attach to the instances. See
'main.tf:local.service_account' for the default.
EOD
  default     = null
}

variable "shielded_instance_config" {
  type = object({
    enable_integrity_monitoring = bool
    enable_secure_boot          = bool
    enable_vtpm                 = bool
  })
  description = <<EOD
Shielded VM configuration for the instance. Note: not used unless
enable_shielded_vm is 'true'.
* enable_integrity_monitoring : Compare the most recent boot measurements to the
  integrity policy baseline and return a pair of pass/fail results depending on
  whether they match or not.
* enable_secure_boot : Verify the digital signature of all boot components, and
  halt the boot process if signature verification fails.
* enable_vtpm : Use a virtualized trusted platform module, which is a
  specialized computer chip you can use to encrypt objects like keys and
  certificates.
EOD
  default = {
    enable_integrity_monitoring = true
    enable_secure_boot          = true
    enable_vtpm                 = true
  }
}

variable "enable_confidential_vm" {
  type        = bool
  description = "Enable the Confidential VM configuration. Note: the instance image must support option."
  default     = false
}

variable "enable_shielded_vm" {
  type        = bool
  description = "Enable the Shielded VM configuration. Note: the instance image must support option."
  default     = false
}

variable "preemptible" {
  type        = bool
  description = "Allow the instance to be preempted."
  default     = false
}

variable "on_host_maintenance" {
  type        = string
  description = "Instance availability Policy"
  default     = "MIGRATE"
}

variable "enable_oslogin" {
  type        = bool
  description = <<EOD
Enables Google Cloud os-login for user login and authentication for VMs.
See https://cloud.google.com/compute/docs/oslogin
EOD
  default     = true
}

variable "num_instances" {
  type        = number
  description = "Number of instances to create. This value is ignored if static_ips is provided."
  default     = 1
}

################
# SOURCE IMAGE #
################

variable "source_image_project" {
  type        = string
  description = "Project where the source image comes from. If it is not provided, the provider project is used."
  default     = null
}

variable "source_image_family" {
  type        = string
  description = "Source image family."
  default     = null
}

variable "source_image" {
  type        = string
  description = "Source disk image."
  default     = null
}

########
# DISK #
########

variable "disk_type" {
  type        = string
  description = "Boot disk type, can be either pd-ssd, local-ssd, or pd-standard."
  default     = "pd-standard"
}

variable "disk_size_gb" {
  type        = number
  description = "Boot disk size in GB."
  default     = 100
}

variable "disk_auto_delete" {
  type        = bool
  description = "Whether or not the boot disk should be auto-deleted."
  default     = true
}

variable "additional_disks" {
  type = list(object({
    disk_name    = string
    device_name  = string
    disk_type    = string
    disk_size_gb = number
    disk_labels  = map(string)
    auto_delete  = bool
    boot         = bool
  }))
  description = "List of maps of disks."
  default     = []
}
