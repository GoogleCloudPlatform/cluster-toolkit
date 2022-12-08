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

# Most variables have been sourced and modified from the SchedMD/slurm-gcp
# github repository: https://github.com/SchedMD/slurm-gcp/tree/v5.1.0

variable "project_id" {
  type        = string
  description = "Project ID to create resources in."
}

variable "labels" {
  type        = map(string)
  description = "Labels, provided as a map."
  default     = {}
}

variable "disable_smt" {
  type        = bool
  description = "Disables Simultaneous Multi-Threading (SMT) on instance."
  default     = true
}

variable "deployment_name" {
  description = "Name of the deployment."
  type        = string
}

variable "disable_login_public_ips" {
  description = "If set to false. The login will have a random public IP assigned to it. Ignored if access_config is set."
  type        = bool
  default     = true
}

variable "slurm_cluster_name" {
  type        = string
  description = "Cluster name, used for resource naming and slurm accounting. If not provided it will default to the first 8 characters of the deployment name (removing any invalid characters)."
  default     = null
}

variable "controller_instance_id" {
  description = <<-EOD
    The server-assigned unique identifier of the controller instance. This value
    must be supplied as an output of the controller module, typically via `use`.
    EOD
  type        = string
}

variable "can_ip_forward" {
  type        = bool
  description = "Enable IP forwarding, for NAT instances for example."
  default     = false
}

variable "network_self_link" {
  type        = string
  description = "Network to deploy to. Either network_self_link or subnetwork_self_link must be specified."
  default     = null
}

variable "subnetwork_self_link" {
  type        = string
  description = "Subnet to deploy to. Either network_self_link or subnetwork_self_link must be specified."
  default     = null
}

variable "subnetwork_project" {
  type        = string
  description = "The project that subnetwork belongs to."
  default     = null
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
  description = <<-EOD
    Zone where the instances should be created. If not specified, instances will be
    spread across available zones in the region.
    EOD
  default     = null
}

variable "metadata" {
  type        = map(string)
  description = "Metadata, provided as a map."
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
  description = <<-EOD
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
  description = <<-EOD
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
  description = <<-EOD
    Service account to attach to the login instance. If not set, the
    default compute service account for the given project will be used with the
    "https://www.googleapis.com/auth/cloud-platform" scope.
    EOD
  default     = null
}

variable "shielded_instance_config" {
  type = object({
    enable_integrity_monitoring = bool
    enable_secure_boot          = bool
    enable_vtpm                 = bool
  })
  description = <<-EOD
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
  description = "Instance availability Policy."
  default     = "MIGRATE"
}

variable "enable_oslogin" {
  type        = bool
  description = <<-EOD
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

variable "startup_script" {
  description = "Startup script that will be used by the login node VM."
  type        = string
  default     = ""
}

variable "instance_image" {
  description = <<-EOD
    Defines the image that will be used in the Slurm login node VM instances. This
    value is overridden if any of `source_image`, `source_image_family` or
    `source_image_project` are set.

    Expected Fields:
    name: The name of the image. Mutually exclusive with family.
    family: The image family to use. Mutually exclusive with name.
    project: The project where the image is hosted.

    Custom images must comply with Slurm on GCP requirements; it is highly
    advised to use the packer templates provided by Slurm on GCP when
    constructing custom slurm images.

    More information can be found in the slurm-gcp docs:
    https://github.com/SchedMD/slurm-gcp/blob/5.3.0/docs/images.md#public-image.
    EOD
  type        = map(string)
  default = {
    family  = "schedmd-v5-slurm-22-05-6-hpc-centos-7"
    project = "projects/schedmd-slurm-public/global/images/family"
  }

  validation {
    condition = length(var.instance_image) == 0 || (
    can(var.instance_image["family"]) || can(var.instance_image["name"])) == can(var.instance_image["project"])
    error_message = "The \"project\" is required if \"family\" or \"name\" are provided in var.instance_image."
  }
  validation {
    condition     = length(var.instance_image) == 0 || can(var.instance_image["family"]) != can(var.instance_image["name"])
    error_message = "Exactly one of \"family\" and \"name\" must be provided in var.instance_image."
  }
}

variable "source_image_project" {
  type        = string
  description = "The hosting the custom VM image. It is recommended to use `instance_image` instead."
  default     = ""
}

variable "source_image_family" {
  type        = string
  description = "The custom VM image family. It is recommended to use `instance_image` instead."
  default     = ""
}

variable "source_image" {
  type        = string
  description = "The custom VM image. It is recommended to use `instance_image` instead."
  default     = ""
}

variable "disk_type" {
  type        = string
  description = "Boot disk type, can be either pd-ssd, local-ssd, or pd-standard."
  default     = "pd-standard"

  validation {
    condition     = contains(["pd-ssd", "local-ssd", "pd-standard"], var.disk_type)
    error_message = "Variable disk_type must be one of pd-ssd, local-ssd, or pd-standard."
  }
}

variable "disk_size_gb" {
  type        = number
  description = "Boot disk size in GB."
  default     = 50
}

variable "disk_auto_delete" {
  type        = bool
  description = "Whether or not the boot disk should be auto-deleted."
  default     = true
}

variable "disk_labels" {
  description = "Labels specific to the boot disk. These will be merged with var.labels."
  type        = map(string)
  default     = {}
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
