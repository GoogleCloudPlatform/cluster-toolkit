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
  description = "Type of boot disk to create for the cluster controller node"
  type        = string
  default     = "pd-standard"
}

variable "controller_image" {
  description = "Slurm image to use for the controller instance"
  type        = string
  default     = "projects/schedmd-slurm-public/global/images/family/schedmd-slurm-20-11-7-hpc-centos-7"
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
    "https://www.googleapis.com/auth/logging.write"
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
  default     = false
}

variable "labels" {
  description = "Labels to add to controller instance. List of key key, value pairs."
  type        = any
  default     = {}
}

variable "login_network_storage" {
  description = "An array of network attached storage mounts to be configured on the login and controller instances."
  type = list(object({
    server_ip     = string,
    remote_mount  = string,
    local_mount   = string,
    fs_type       = string,
    mount_options = string
  }))
  default = []
}

variable "login_node_count" {
  description = "Number of login nodes in the cluster"
  type        = number
  default     = 0
}

variable "controller_machine_type" {
  description = "Compute Platform machine type to use in controller node creation"
  type        = string
  default     = "n2-standard-2"
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
  description = " An array of network attached storage mounts to be configured on all instances."
  type = list(object({
    server_ip     = string,
    remote_mount  = string,
    local_mount   = string,
    fs_type       = string,
    mount_options = string
  }))
  default = []
}

variable "partitions" {
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
    preemptible_bursting = bool,
    vpc_subnet           = string,
    exclusive            = bool,
    enable_placement     = bool,
    regional_capacity    = bool,
    regional_policy      = any,
    instance_template    = string,
    static_node_count    = number
  }))
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
  default     = ["https://www.googleapis.com/auth/cloud-platform"]
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
