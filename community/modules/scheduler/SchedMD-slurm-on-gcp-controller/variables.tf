#
# Copyright 2019 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

variable "boot_disk_size" {
  description = "Size of boot disk to create for the cluster controller node"
  type        = number
  default     = 50
}

variable "boot_disk_type" {
  description = <<-EOT
  Type of boot disk to create for the cluster controller node.
  Choose from: pd-ssd, pd-standard, pd-balanced, pd-extreme.
  pd-ssd is recommended if the controller is hosting the SlurmDB and NFS share.
  If SlurmDB and NFS share are not running on the controller, pd-standard is
  recommended. See "Controller configuration recommendations" in the Slurm on
  Google Cloud User Guide for more information:
  https://goo.gle/slurm-gcp-user-guide
  EOT
  type        = string
  default     = "pd-ssd"
}

variable "instance_image" {
  description = <<-EOD
    Slurm image to use for the controller instance.
    
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

variable "controller_instance_template" {
  description = "Instance template to use to create controller instance"
  type        = string
  default     = null
}

variable "cluster_name" {
  description = "Name of the cluster"
  type        = string
  default     = null
}

variable "deployment_name" {
  description = "Name of the deployment"
  type        = string
}

variable "compute_node_scopes" {
  description = "Scopes to apply to compute nodes."
  type        = list(string)
  default = [
    "https://www.googleapis.com/auth/monitoring.write",
    "https://www.googleapis.com/auth/logging.write",
    "https://www.googleapis.com/auth/devstorage.read_only",
  ]
}

variable "compute_node_service_account" {
  description = "Service Account for compute nodes."
  type        = string
  default     = null
}

variable "disable_controller_public_ips" {
  description = "If set to true, create Cloud NAT gateway and enable IAP FW rules"
  type        = bool
  default     = false
}

variable "disable_compute_public_ips" {
  description = "If set to true, create Cloud NAT gateway and enable IAP FW rules"
  type        = bool
  default     = true
}

variable "labels" {
  description = "Labels to add to controller instance.  Key-value pairs."
  type        = map(string)
  default     = {}
}

variable "login_node_count" {
  description = "Number of login nodes in the cluster"
  type        = number
  default     = 0
}

variable "controller_machine_type" {
  description = <<-EOT
  Compute Platform machine type to use in controller node creation. `c2-standard-4`
  is recommended for clusters up to 50 nodes, for larger clusters see
  "Controller configuration recommendations" in the Slurm on Google Cloud User
  Guide: https://goo.gle/slurm-gcp-user-guide
  EOT
  type        = string
  default     = "c2-standard-4"
}

variable "munge_key" {
  description = "Specific munge key to use"
  type        = any
  default     = null
}

variable "jwt_key" {
  description = "Specific libjwt key to use"
  type        = any
  default     = null
}

variable "network_storage" {
  description = "An array of network attached storage mounts to be configured on all instances."
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

variable "partition" {
  description = "An array of configurations for specifying multiple machine types residing in their own Slurm partitions."
  type = list(object({
    name                 = string,
    machine_type         = string,
    max_node_count       = number,
    zone                 = string,
    image                = string,
    image_hyperthreads   = bool,
    compute_disk_type    = string,
    compute_disk_size_gb = number,
    compute_labels       = any,
    cpu_platform         = string,
    gpu_type             = string,
    gpu_count            = number,
    network_storage = list(object({
      server_ip     = string,
      remote_mount  = string,
      local_mount   = string,
      fs_type       = string,
      mount_options = string
    })),
    preemptible_bursting = string,
    vpc_subnet           = string,
    exclusive            = bool,
    enable_placement     = bool,
    regional_capacity    = bool,
    regional_policy      = any,
    instance_template    = string,
    bandwidth_tier       = string,
    static_node_count    = number
  }))
}

variable "controller_startup_script" {
  description = "Custom startup script to run on the controller"
  type        = string
  default     = null
}

variable "compute_startup_script" {
  description = "Custom startup script to run on the compute nodes"
  type        = string
  default     = null
}

variable "startup_script" {
  description = <<EOT
  Custom startup script to run on compute nodes and controller. 
  `controller_startup_script` for the controller and `compute_startup_script` for compute nodes take presidence if specified.
  This variable allows Slurm to [use](https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/modules#use-optional) the [startup_script](https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/modules/scripts/startup-script) module.
  EOT
  type        = string
  default     = null
}

variable "project_id" {
  description = "Compute Platform project that will host the Slurm cluster"
  type        = string
}

variable "region" {
  description = "Compute Platform region where the Slurm cluster will be located"
  type        = string
}

variable "controller_secondary_disk" {
  description = "Create secondary disk mounted to controller node"
  type        = bool
  default     = false
}

variable "controller_secondary_disk_size" {
  description = "Size of disk for the secondary disk"
  type        = number
  default     = 100
}

variable "controller_secondary_disk_type" {
  description = "Disk type (pd-ssd or pd-standard) for secondary disk"
  type        = string
  default     = "pd-ssd"
}

variable "shared_vpc_host_project" {
  description = "Host project of shared VPC"
  type        = string
  default     = null
}

variable "controller_scopes" {
  description = "Scopes to apply to the controller"
  type        = list(string)
  default = [
    "https://www.googleapis.com/auth/cloud-platform",
    "https://www.googleapis.com/auth/devstorage.read_only",
  ]
}

variable "controller_service_account" {
  description = "Service Account for the controller"
  type        = string
  default     = null
}

variable "subnetwork_name" {
  description = "The name of the pre-defined VPC subnet you want the nodes to attach to based on Region."
  type        = string
  default     = null
}

variable "zone" {
  description = "Compute Platform zone where the servers will be located"
  type        = string
}

variable "suspend_time" {
  description = "Idle time (in sec) to wait before nodes go away"
  type        = number
  default     = 300
}

variable "intel_select_solution" {
  description = "Configure the cluster to meet the performance requirement of the Intel Select Solution"
  type        = string
  default     = null
}

variable "cloudsql" {
  description = "Define an existing CloudSQL instance to use instead of instance-local MySQL"
  type = object({
    server_ip = string,
    user      = string,
    password  = string,
    db_name   = string
  })
  default = null
}
