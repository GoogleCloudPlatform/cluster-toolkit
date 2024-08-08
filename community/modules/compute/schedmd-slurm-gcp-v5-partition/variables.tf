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
# github repository: https://github.com/GoogleCloudPlatform/slurm-gcp/tree/v5

variable "deployment_name" {
  description = "Name of the deployment."
  type        = string
}

variable "slurm_cluster_name" {
  type        = string
  description = "Cluster name, used for resource naming and slurm accounting. If not provided it will default to the first 8 characters of the deployment name (removing any invalid characters)."
  default     = null
}

variable "project_id" {
  description = "Project in which the HPC deployment will be created."
  type        = string
}

variable "region" {
  description = "The default region for Cloud resources."
  type        = string
}

variable "zone" {
  description = "Zone in which to create compute VMs. Additional zones in the same region can be specified in var.zones."
  type        = string
}

variable "zones" {
  description = <<-EOD
    Additional nodes in which to allow creation of partition nodes. Google Cloud
    will find zone based on availability, quota and reservations.
    EOD
  type        = set(string)
  default     = []

  validation {
    condition = alltrue([
      for x in var.zones : length(regexall("^[a-z]+-[a-z]+[0-9]-[a-z]$", x)) > 0
    ])
    error_message = "A value in var.zones is not a valid zone (example: us-central1-f)."
  }
}

variable "zone_target_shape" {
  description = <<EOD
Strategy for distributing VMs across zones in a region.
ANY
  GCE picks zones for creating VM instances to fulfill the requested number of VMs
  within present resource constraints and to maximize utilization of unused zonal
  reservations.
ANY_SINGLE_ZONE (default)
  GCE always selects a single zone for all the VMs, optimizing for resource quotas,
  available reservations and general capacity.
BALANCED
  GCE prioritizes acquisition of resources, scheduling VMs in zones where resources
  are available while distributing VMs as evenly as possible across allowed zones
  to minimize the impact of zonal failure.
EOD
  type        = string
  default     = "ANY_SINGLE_ZONE"
  validation {
    condition     = contains(["ANY", "ANY_SINGLE_ZONE", "BALANCED"], var.zone_target_shape)
    error_message = "Allowed values for zone_target_shape are \"ANY\", \"ANY_SINGLE_ZONE\", or \"BALANCED\"."
  }
}

variable "partition_name" {
  description = "The name of the slurm partition."
  type        = string

  validation {
    condition     = can(regex("^[a-z](?:[a-z0-9]{0,6})$", var.partition_name))
    error_message = "Variable 'partition_name' must be composed of only alphanumeric characters, start with a letter and be 7 characters or less. Regexp: '^[a-z](?:[a-z0-9]{0,6})$'."
  }
}

variable "partition_conf" {
  description = <<-EOD
    Slurm partition configuration as a map.
    See https://slurm.schedmd.com/slurm.conf.html#SECTION_PARTITION-CONFIGURATION
    EOD
  type        = map(string)
  default     = {}
}

variable "startup_script" {
  description = "Startup script that will be used by the partition VMs."
  type        = string
  default     = ""
}

variable "partition_startup_scripts_timeout" {
  description = <<-EOD
    The timeout (seconds) applied to the partition startup script. If
    any script exceeds this timeout, then the instance setup process is considered
    failed and handled accordingly.

    NOTE: When set to 0, the timeout is considered infinite and thus disabled.
    EOD
  type        = number
  default     = 300
}

variable "is_default" {
  description = <<-EOD
    Sets this partition as the default partition by updating the partition_conf.
    If "Default" is already set in partition_conf, this variable will have no effect.
    EOD
  type        = bool
  default     = false
}

variable "subnetwork_self_link" {
  type        = string
  description = "Subnet to deploy to."
  default     = null
}

variable "subnetwork_project" {
  description = "The project the subnetwork belongs to."
  type        = string
  default     = ""
}

variable "exclusive" {
  description = "Exclusive job access to nodes."
  type        = bool
  default     = true
}

variable "enable_placement" {
  description = "Enable placement groups."
  type        = bool
  default     = true
}

variable "enable_reconfigure" {
  description = <<-EOD
    Enables automatic Slurm reconfigure on when Slurm configuration changes (e.g.
    slurm.conf.tpl, partition details). Compute instances and resource policies
    (e.g. placement groups) will be destroyed to align with new configuration.

    NOTE: Requires Python and Google Pub/Sub API.

    *WARNING*: Toggling this will impact the running workload. Deployed compute nodes
    will be destroyed and their jobs will be requeued.
    EOD
  type        = bool
  default     = false
}

variable "network_storage" {
  description = "An array of network attached storage mounts to be configured on the partition compute nodes."
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

variable "node_groups" {
  description = <<-EOT
    A list of node groups associated with this partition. See
    schedmd-slurm-gcp-v5-node-group for more information on defining a node
    group in a blueprint.
    EOT
  type = list(object({
    node_count_static      = number
    node_count_dynamic_max = number
    group_name             = string
    node_conf              = map(string)
    access_config = list(object({
      nat_ip       = string
      network_tier = string
    }))
    additional_disks = list(object({
      disk_name    = string
      device_name  = string
      disk_size_gb = number
      disk_type    = string
      disk_labels  = map(string)
      auto_delete  = bool
      boot         = bool
    }))
    additional_networks = list(object({
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
    }))
    bandwidth_tier         = string
    can_ip_forward         = bool
    disable_smt            = bool
    disk_auto_delete       = bool
    disk_labels            = map(string)
    disk_size_gb           = number
    disk_type              = string
    enable_confidential_vm = bool
    enable_oslogin         = bool
    enable_shielded_vm     = bool
    enable_spot_vm         = bool
    gpu = object({
      count = number
      type  = string
    })
    instance_template    = string
    labels               = map(string)
    machine_type         = string
    maintenance_interval = string
    metadata             = map(string)
    min_cpu_platform     = string
    on_host_maintenance  = string
    preemptible          = bool
    reservation_name     = string
    service_account = object({
      email  = string
      scopes = list(string)
    })
    shielded_instance_config = object({
      enable_integrity_monitoring = bool
      enable_secure_boot          = bool
      enable_vtpm                 = bool
    })
    spot_instance_config = object({
      termination_action = string
    })
    source_image_family  = string
    source_image_project = string
    source_image         = string
    tags                 = list(string)
  }))
  default = []
}
