# Copyright 2025 Google LLC
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


variable "nodeset" {
  description = "Nodeset definition"
  # TODO: remove optional & defaults from fields, since they SHOULD be properly set by user-facing nodeset module and not here.
  type = object({
    node_count_static      = optional(number, 0)
    node_count_dynamic_max = optional(number, 1)
    node_conf              = optional(map(string), {})
    nodeset_name           = string
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
    bandwidth_tier                   = optional(string, "platform_default")
    can_ip_forward                   = optional(bool, false)
    disk_auto_delete                 = optional(bool, true)
    disk_labels                      = optional(map(string), {})
    disk_resource_manager_tags       = optional(map(string), {})
    disk_size_gb                     = optional(number)
    disk_type                        = optional(string)
    enable_confidential_vm           = optional(bool, false)
    enable_placement                 = optional(bool, false)
    placement_max_distance           = optional(number, null)
    enable_oslogin                   = optional(bool, true)
    enable_shielded_vm               = optional(bool, false)
    enable_maintenance_reservation   = optional(bool, false)
    enable_opportunistic_maintenance = optional(bool, false)
    gpu = optional(object({
      count = number
      type  = string
    }))
    dws_flex = object({
      enabled          = bool
      max_run_duration = number
      use_job_duration = bool
      use_bulk_insert  = bool
    })
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
    maintenance_interval     = optional(string)
    instance_properties_json = string
    metadata                 = optional(map(string), {})
    min_cpu_platform         = optional(string)
    network_tier             = optional(string, "STANDARD")
    network_storage = optional(list(object({
      server_ip     = string
      remote_mount  = string
      local_mount   = string
      fs_type       = string
      mount_options = string
    })), [])
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
    subnetwork_self_link = string
    additional_networks = optional(list(object({
      network            = string
      subnetwork         = string
      subnetwork_project = string
      network_ip         = string
      nic_type           = string
      stack_type         = string
      queue_count        = number
      access_config = list(object({
        nat_ip       = string
        network_tier = string
      }))
      ipv6_access_config = list(object({
        network_tier = string
      }))
      alias_ip_range = list(object({
        ip_cidr_range         = string
        subnetwork_range_name = string
      }))
    })))
    access_config = optional(list(object({
      nat_ip       = string
      network_tier = string
    })))
    spot               = optional(bool, false)
    tags               = optional(list(string), [])
    termination_action = optional(string)
    reservation_name   = optional(string)
    future_reservation = string

    zone_target_shape = string
    zone_policy_allow = set(string)
    zone_policy_deny  = set(string)
  })
}

variable "startup_scripts" {
  description = "List of scripts to be ran on VMs startup."
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
