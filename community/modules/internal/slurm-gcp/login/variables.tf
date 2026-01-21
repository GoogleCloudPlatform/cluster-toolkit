# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

variable "project_id" {
  type        = string
  description = "Project ID to create resources in."
}

variable "slurm_cluster_name" {
  type        = string
  description = "Cluster name"
}

variable "slurm_bucket_path" {
  type        = string
  description = "GCS Bucket URI of Slurm cluster file storage."
}


variable "slurm_bucket_name" {
  type        = string
  description = "Name of the bucket for configs"
}

variable "slurm_bucket_dir" {
  type        = string
  description = "Path to directory in the bucket for configs"
}


variable "universe_domain" {
  description = "Domain address for alternate API universe"
  type        = string
  default     = "googleapis.com"
}

variable "login_nodes" {
  description = "Slurm login instance definitions."
  type = object({
    group_name = string
    access_config = optional(list(object({
      nat_ip       = string
      network_tier = string
    })))
    additional_disks = optional(list(object({
      disk_name                  = optional(string)
      device_name                = optional(string)
      disk_size_gb               = optional(number)
      disk_type                  = optional(string)
      disk_labels                = optional(map(string), {})
      auto_delete                = optional(bool, true)
      boot                       = optional(bool, false)
      disk_resource_manager_tags = optional(map(string), {})
    })), [])
    additional_networks = optional(list(object({
      access_config = optional(list(object({
        nat_ip       = string
        network_tier = string
      })), [])
      alias_ip_range = optional(list(object({
        ip_cidr_range         = string
        subnetwork_range_name = string
      })), [])
      ipv6_access_config = optional(list(object({
        network_tier = string
      })), [])
      network            = optional(string)
      network_ip         = optional(string, "")
      nic_type           = optional(string)
      queue_count        = optional(number)
      stack_type         = optional(string)
      subnetwork         = optional(string)
      subnetwork_project = optional(string)
    })), [])
    bandwidth_tier             = optional(string, "platform_default")
    can_ip_forward             = optional(bool, false)
    disk_auto_delete           = optional(bool, true)
    disk_labels                = optional(map(string), {})
    disk_resource_manager_tags = optional(map(string), {})
    disk_size_gb               = optional(number)
    disk_type                  = optional(string, "n1-standard-1")
    enable_confidential_vm     = optional(bool, false)
    enable_oslogin             = optional(bool, true)
    enable_shielded_vm         = optional(bool, false)
    gpu = optional(object({
      count = number
      type  = string
    }))
    labels       = optional(map(string), {})
    machine_type = optional(string)
    advanced_machine_features = object({
      enable_nested_virtualization = optional(bool)
      threads_per_core             = optional(number)
      turbo_mode                   = optional(string)
      visible_core_count           = optional(number)
      performance_monitoring_unit  = optional(string)
      enable_uefi_networking       = optional(bool)
    })
    metadata              = optional(map(string), {})
    min_cpu_platform      = optional(string)
    num_instances         = optional(number, 1)
    on_host_maintenance   = optional(string)
    preemptible           = optional(bool, false)
    region                = optional(string)
    resource_manager_tags = optional(map(string), {})
    service_account = optional(object({
      email  = optional(string)
      scopes = optional(list(string), ["https://www.googleapis.com/auth/cloud-platform"])
    }))
    shielded_instance_config = optional(object({
      enable_integrity_monitoring = optional(bool, true)
      enable_secure_boot          = optional(bool, true)
      enable_vtpm                 = optional(bool, true)
    }))
    source_image_family  = optional(string)
    source_image_project = optional(string)
    source_image         = optional(string)
    static_ips           = optional(list(string), [])
    subnetwork           = string
    spot                 = optional(bool, false)
    tags                 = optional(list(string), [])
    zone                 = optional(string)
    termination_action   = optional(string)
  })
}


variable "startup_scripts" {
  description = "List of scripts to be ran on login VMs startup."
  type = list(object({
    filename = string
    content  = string
  }))
  default = []
}

variable "startup_scripts_timeout" {
  description = <<EOD
The timeout (seconds) applied to each startup script. If any script exceeds this timeout, 
then the instance setup process is considered failed and handled accordingly.

NOTE: When set to 0, the timeout is considered infinite and thus disabled.
EOD
  type        = number
  default     = 300
}

variable "network_storage" {
  description = <<EOD
Storage to mounted on login instances
- server_ip     : Address of the storage server.
- remote_mount  : The location in the remote instance filesystem to mount from.
- local_mount   : The location on the instance filesystem to mount to.
- fs_type       : Filesystem type (e.g. "nfs").
- mount_options : Options to mount with.
EOD
  type = list(object({
    server_ip     = string
    remote_mount  = string
    local_mount   = string
    fs_type       = string
    mount_options = string
  }))
  default = []
}

variable "replace_trigger" {
  description = "Trigger value to replace the instances."
  type        = string
  default     = ""
}

variable "internal_startup_script" {
  description = "FOR INTERNAL TOOLKIT USAGE ONLY."
  type        = string
  default     = null
}
