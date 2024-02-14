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

variable "boot_disk_size" {
  description = "Size of boot disk to create for the cluster login node"
  type        = number
  default     = 20
}

variable "boot_disk_type" {
  description = "Type of boot disk to create for the cluster login node"
  type        = string
  default     = "pd-standard"
}

variable "instance_image" {
  description = <<-EOD
    Disk OS image with Slurm preinstalled to use for login node.
    
    Expected Fields:
    name: The name of the image. Mutually exclusive with family.
    family: The image family to use. Mutually exclusive with name.
    project: The project where the image is hosted.
    Custom images must comply with Slurm on GCP requirements.
    EOD
  type        = map(string)
  default = {
    project = "schedmd-slurm-public"
    family  = "schedmd-slurm-21-08-8-hpc-centos-7"
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

variable "login_instance_template" {
  description = "Instance template to use to create controller instance"
  type        = string
  default     = null
}

variable "cluster_name" {
  description = "Name of the cluster"
  type        = string
  default     = null
}

variable "controller_name" {
  description = "FQDN or IP address of the controller node"
  type        = string
}

variable "controller_secondary_disk" {
  description = "Create secondary disk mounted to controller node"
  type        = bool
  default     = false
}

variable "deployment_name" {
  description = "Name of the deployment"
  type        = string
}

variable "disable_login_public_ips" {
  description = "If set to true, create Cloud NAT gateway and enable IAP FW rules"
  type        = bool
  default     = false
}

variable "labels" {
  description = "Labels to add to login instances. Key-value pairs."
  type        = map(string)
  default     = {}
}

variable "login_machine_type" {
  description = "Machine type to use for login node instances."
  type        = string
  default     = "n2-standard-2"
}

variable "munge_key" {
  description = "Specific munge key to use"
  type        = any
  default     = null
}

variable "network_storage" {
  description = " An array of network attached storage mounts to be configured on all instances."
  type = list(object({
    server_ip             = string,
    remote_mount          = string,
    local_mount           = string,
    fs_type               = string,
    mount_options         = string,
    client_install_runner = map(string)
    mount_runner          = map(string)
  }))
  default = []
}

variable "login_node_count" {
  description = "Number of login nodes in the cluster"
  type        = number
  default     = 1
}

variable "region" {
  description = "Compute Platform region where the Slurm cluster will be located"
  type        = string
}

variable "login_scopes" {
  description = "Scopes to apply to login nodes."
  type        = list(string)
  default = [
    "https://www.googleapis.com/auth/monitoring.write",
    "https://www.googleapis.com/auth/logging.write",
    "https://www.googleapis.com/auth/devstorage.read_only",
  ]
}

variable "login_service_account" {
  description = "Service Account for compute nodes."
  type        = string
  default     = null
}

variable "shared_vpc_host_project" {
  description = "Host project of shared VPC"
  type        = string
  default     = null
}

variable "subnet_depend" {
  description = "Used as a dependency between the network and instances"
  type        = string
  default     = ""
}

variable "subnetwork_name" {
  description = "The name of the pre-defined VPC subnet you want the nodes to attach to based on Region."
  type        = string
  default     = null
}

variable "zone" {
  description = "Compute Platform zone where the notebook server will be located"
  type        = string
}

variable "login_startup_script" {
  description = "Custom startup script to run on the login node"
  type        = string
  default     = null
}

variable "startup_script" {
  description = <<EOT
  Custom startup script to run on the login node. 
  Will be ignored if `login_startup_script` is specified.
  This variable allows Slurm to [use](https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/modules#use-optional) the [startup_script](https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/modules/scripts/startup-script) module.
  EOT
  type        = string
  default     = null
}
