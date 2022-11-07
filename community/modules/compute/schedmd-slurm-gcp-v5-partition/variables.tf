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
  description = "Zone in which to create all compute VMs. If `zone_policy_deny` or `zone_policy_allow` are set, the `zone` variable will be ignored."
  type        = string
  default     = null
}

variable "zone_policy_allow" {
  description = <<-EOD
    Partition nodes will prefer to be created in the listed zones. If a zone appears
    in both zone_policy_allow and zone_policy_deny, then zone_policy_deny will take
    priority for that zone.
    EOD
  type        = set(string)
  default     = []

  validation {
    condition = alltrue([
      for x in var.zone_policy_allow : length(regexall("^[a-z]+-[a-z]+[0-9]-[a-z]$", x)) > 0
    ])
    error_message = "A provided zone in zone_policy_allow is not a valid zone (Regexp: '^[a-z]+-[a-z]+[0-9]-[a-z]$')."
  }
}

variable "zone_policy_deny" {
  description = <<-EOD
    Partition nodes will not be created in the listed zones. If a zone appears in
    both zone_policy_allow and zone_policy_deny, then zone_policy_deny will take
    priority for that zone.
    EOD
  type        = set(string)
  default     = []

  validation {
    condition = alltrue([
      for x in var.zone_policy_deny : length(regexall("^[a-z]+-[a-z]+[0-9]-[a-z]$", x)) > 0
    ])
    error_message = "A provided zone in zone_policy_deny is not a valid zone (Regexp '^[a-z]+-[a-z]+[0-9]-[a-z]$')."
  }
}

variable "partition_name" {
  description = "The name of the slurm partition."
  type        = string
}

variable "partition_conf" {
  description = <<-EOD
    Slurm partition configuration as a map.
    See https://slurm.schedmd.com/slurm.conf.html#SECTION_PARTITION-CONFIGURATION
    EOD
  type        = map(string)
  default     = {}
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
    server_ip     = string,
    remote_mount  = string,
    local_mount   = string,
    fs_type       = string,
    mount_options = string
  }))
  default = []
}

variable "node_groups" {
  description = <<-EOT
    **Preview: This variable is still in development** A list of node groups
    associated with this partition.
    The default node group will be prepended to this list based on other input
    variables to this module.
    EOT
  type = list(object({
    node_count_static      = number
    node_count_dynamic_max = number
    group_name             = string
    node_conf              = map(string)
    additional_disks = list(object({
      disk_name    = string
      device_name  = string
      disk_size_gb = number
      disk_type    = string
      disk_labels  = map(string)
      auto_delete  = bool
      boot         = bool
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
    instance_template   = string
    labels              = map(string)
    machine_type        = string
    metadata            = map(string)
    min_cpu_platform    = string
    on_host_maintenance = string
    preemptible         = bool
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

## Default node group variables have been deprecated from the partition module.
## Use the schedmd-slurm-gcp-v5-node-group module and node_groups variable for
## defining any of the following values moving forward.

variable "machine_type" {
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  type        = string
  default     = null

  validation {
    condition     = var.machine_type == null
    error_message = "The variable var.machine_type in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "metadata" {
  type        = map(string)
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  default     = null

  validation {
    condition     = var.metadata == null
    error_message = "The variable var.metadata in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "node_count_static" {
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  type        = number
  default     = null

  validation {
    condition     = var.node_count_static == null
    error_message = "The variable var.node_count_static in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "node_conf" {
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  type        = map(any)
  default     = null

  validation {
    condition     = var.node_conf == null
    error_message = "The variable var.node_conf in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "node_count_dynamic_max" {
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  type        = number
  default     = null

  validation {
    condition     = var.node_count_dynamic_max == null
    error_message = "The variable var.node_count_dynamic_max in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "source_image_project" {
  type        = string
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  default     = null

  validation {
    condition     = var.source_image_project == null
    error_message = "The variable var.source_image_project in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "source_image_family" {
  type        = string
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  default     = null

  validation {
    condition     = var.source_image_family == null
    error_message = "The variable var.source_image_family in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "source_image" {
  type        = string
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  default     = null

  validation {
    condition     = var.source_image == null
    error_message = "The variable var.source_image in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "tags" {
  type        = list(string)
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  default     = null

  validation {
    condition     = var.tags == null
    error_message = "The variable var.tags in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "disk_type" {
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  type        = string
  default     = null

  validation {
    condition     = var.disk_type == null
    error_message = "The variable var.disk_type in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "disk_size_gb" {
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  type        = number
  default     = null

  validation {
    condition     = var.disk_size_gb == null
    error_message = "The variable var.disk_size_gb in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "disk_auto_delete" {
  type        = bool
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  default     = null

  validation {
    condition     = var.disk_auto_delete == null
    error_message = "The variable var.disk_auto_delete in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "additional_disks" {
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  type = list(object({
    disk_name    = string
    device_name  = string
    disk_size_gb = number
    disk_type    = string
    disk_labels  = map(string)
    auto_delete  = bool
    boot         = bool
  }))
  default = null

  validation {
    condition     = var.additional_disks == null
    error_message = "The variable var.additional_disks in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "enable_confidential_vm" {
  type        = bool
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  default     = null

  validation {
    condition     = var.enable_confidential_vm == null
    error_message = "The variable var.enable_confidential_vm in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "enable_shielded_vm" {
  type        = bool
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  default     = null

  validation {
    condition     = var.enable_shielded_vm == null
    error_message = "The variable var.enable_shielded_vm in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "enable_oslogin" {
  type        = bool
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  default     = null

  validation {
    condition     = var.enable_oslogin == null
    error_message = "The variable var.enable_oslogin in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "can_ip_forward" {
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  type        = bool
  default     = null

  validation {
    condition     = var.can_ip_forward == null
    error_message = "The variable var.can_ip_forward in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "disable_smt" {
  type        = bool
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  default     = null

  validation {
    condition     = var.disable_smt == null
    error_message = "The variable var.disable_smt in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

# Not validating, as it's going to be set automatically by ghpc.
# tflint-ignore: terraform_unused_declarations
variable "labels" {
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  type        = any
  default     = null
}

variable "min_cpu_platform" {
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  type        = string
  default     = null

  validation {
    condition     = var.min_cpu_platform == null
    error_message = "The variable var.min_cpu_platform in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "on_host_maintenance" {
  type        = string
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  default     = null

  validation {
    condition     = var.on_host_maintenance == null
    error_message = "The variable var.on_host_maintenance in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "gpu" {
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  type = object({
    count = number,
    type  = string
  })
  default = null

  validation {
    condition     = var.gpu == null
    error_message = "The variable var.gpu in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "preemptible" {
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  type        = string
  default     = null

  validation {
    condition     = var.preemptible == null
    error_message = "The variable var.preemptible in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "service_account" {
  type = object({
    email  = string
    scopes = set(string)
  })
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  default     = null

  validation {
    condition     = var.service_account == null
    error_message = "The variable var.service_account in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "shielded_instance_config" {
  type = object({
    enable_integrity_monitoring = bool
    enable_secure_boot          = bool
    enable_vtpm                 = bool
  })
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  default     = null

  validation {
    condition     = var.shielded_instance_config == null
    error_message = "The variable var.shielded_instance_config in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}


variable "enable_spot_vm" {
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  type        = bool
  default     = null

  validation {
    condition     = var.enable_spot_vm == null
    error_message = "The variable var.enable_spot_vm in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "spot_instance_config" {
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  type = object({
    termination_action = string
  })
  default = null

  validation {
    condition     = var.spot_instance_config == null
    error_message = "The variable var.spot_instance_config in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}

variable "bandwidth_tier" {
  description = "Deprecated: Use the schedmd-slurm-gcp-v5-node-group module for defining node groups instead."
  type        = string
  default     = null

  validation {
    condition     = var.bandwidth_tier == null
    error_message = "The variable var.bandwidth_tier in schedmd-slurm-gcp-v5-partition is deprecated. Please use the schedmd-slurm-gcp-v5-node-group module instead."
  }
}
