/**
 * Copyright 2023 Google LLC
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
# github repository: https://github.com/SchedMD/slurm-gcp/tree/5.9.1

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
  description = "DEPRECATED: Use `static_ips` variable to assign an internal static ip address."
  default     = null
  validation {
    condition     = var.network_ip == null
    error_message = "network_ip is deprecated. Use static_ips to assign an internal static ip address."
  }
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
  description = "DEPRECATED: use var.guest_accelerator"
  default     = null
  validation {
    condition     = var.gpu == null
    error_message = "var.gpu is deprecated. Use var.guest_accelerator."
  }
}

variable "guest_accelerator" {
  description = "List of the type and count of accelerator cards attached to the instance."
  type = list(object({
    type  = string,
    count = number
  }))
  default  = []
  nullable = false

  validation {
    condition     = length(var.guest_accelerator) <= 1
    error_message = "The Slurm modules supports 0 or 1 models of accelerator card on each node."
  }
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
    - enable_integrity_monitoring : Compare the most recent boot measurements to the
      integrity policy baseline and return a pair of pass/fail results depending on
      whether they match or not.
    - enable_secure_boot : Verify the digital signature of all boot components, and
      halt the boot process if signature verification fails.
    - enable_vtpm : Use a virtualized trusted platform module, which is a
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

variable "instance_template" {
  description = <<-EOD
    Self link to a custom instance template. If set, other VM definition
    variables such as machine_type and instance_image will be ignored in favor
    of the provided instance template.

    For more information on creating custom images for the instance template
    that comply with Slurm on GCP see the "Slurm on GCP Custom Images" section
    in docs/vm-images.md.
    EOD
  type        = string
  default     = null
}

variable "instance_image" {
  description = <<-EOD
    Defines the image that will be used in the Slurm login node VM instances.

    Expected Fields:
    name: The name of the image. Mutually exclusive with family.
    family: The image family to use. Mutually exclusive with name.
    project: The project where the image is hosted.

    For more information on creating custom images that comply with Slurm on GCP
    see the "Slurm on GCP Custom Images" section in docs/vm-images.md.
    EOD
  type        = map(string)
  default = {
    project = "schedmd-slurm-public"
    family  = "slurm-gcp-5-9-hpc-centos-7"
  }

  validation {
    condition     = can(coalesce(var.instance_image.project))
    error_message = "In var.instance_image, the \"project\" field must be a string set to the Cloud project ID."
  }

  validation {
    condition     = can(coalesce(var.instance_image.name)) != can(coalesce(var.instance_image.family))
    error_message = "In var.instance_image, exactly one of \"family\" or \"name\" fields must be set to desired image family or name."
  }
}

variable "instance_image_custom" {
  description = <<-EOD
    A flag that designates that the user is aware that they are requesting
    to use a custom and potentially incompatible image for this Slurm on
    GCP module.

    If the field is set to false, only the compatible families and project
    names will be accepted.  The deployment will fail with any other image
    family or name.  If set to true, no checks will be done.

    See: https://goo.gle/hpc-slurm-images
    EOD
  type        = bool
  default     = false
}

variable "source_image_project" {
  type        = string
  description = "DEPRECATED: Use `instance_image` instead."
  default     = null
  validation {
    condition     = var.source_image_project == null
    error_message = "Variable `source_image_project` is deprecated. Use `instance_image` instead."
  }
}

variable "source_image_family" {
  type        = string
  description = "DEPRECATED: Use `instance_image` instead."
  default     = null
  validation {
    condition     = var.source_image_family == null
    error_message = "Variable `source_image_family` is deprecated. Use `instance_image` instead."
  }
}

variable "source_image" {
  type        = string
  description = "DEPRECATED: Use `instance_image` instead."
  default     = null
  validation {
    condition     = var.source_image == null
    error_message = "Variable `source_image` is deprecated. Use `instance_image` instead."
  }
}

variable "disk_type" {
  type        = string
  description = "Boot disk type, can be either pd-ssd, pd-standard, pd-balanced, or pd-extreme."
  default     = "pd-standard"

  validation {
    condition     = contains(["pd-ssd", "pd-standard", "pd-balanced", "pd-extreme"], var.disk_type)
    error_message = "Variable disk_type must be one of pd-ssd, pd-standard, pd-balanced, or pd-extreme."
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

variable "enable_reconfigure" {
  description = <<EOD
Enables automatic Slurm reconfigure on when Slurm configuration changes (e.g.
slurm.conf.tpl, partition details).

NOTE: Requires Google Pub/Sub API.
EOD
  type        = bool
  default     = false
}

variable "pubsub_topic" {
  description = <<EOD
The cluster pubsub topic created by the controller when enable_reconfigure=true.
EOD
  type        = string
  default     = null
}
