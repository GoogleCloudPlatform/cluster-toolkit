# Copyright 2023 Google LLC
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

variable "name" { # REVIEWER_NOTE: `name` instead of `partition_name` + removed length restriction
  description = "The name of the slurm partition."
  type        = string

  validation {
    condition     = can(regex("^[a-z](?:[a-z0-9]*)$", var.name))
    error_message = "Variable 'name' must be composed of only alphanumeric characters and start with a letter. Regexp: '^[a-z](?:[a-z0-9]*)$'."
  }
}


variable "is_default" {
  description = <<-EOD
    If this is true, jobs submitted without a partition specification will utilize this partition.
    This sets 'Default' in partition_conf.
    See https://slurm.schedmd.com/slurm.conf.html#OPT_Default for details.
    EOD
  type        = bool
  default     = false
}

variable "exclusive" {
  description = "Exclusive job access to nodes."
  type        = bool
  default     = true
}

variable "network_storage" {
  description = "A list of network attached storage mounts to be configured on the partition compute nodes."
  type = list(object({
    server_ip     = string,
    remote_mount  = string,
    local_mount   = string,
    fs_type       = string,
    mount_options = string, # REVIEWER_NOTE: removed runners
  }))
  default = []
}

variable "partition_conf" {
  description = <<-EOD
    Slurm partition configuration as a map.
    See https://slurm.schedmd.com/slurm.conf.html#SECTION_PARTITION-CONFIGURATION
    EOD
  type        = map(string)
  default     = {}
}

variable "resume_timeout" {
  description = <<-EOD
    Maximum time permitted (in seconds) between when a node resume request is issued and when the node is actually available for use.
    If null is given, then a smart default will be chosen depending on nodesets in partition.
    This sets 'ResumeTimeout' in partition_conf.
    See https://slurm.schedmd.com/slurm.conf.html#OPT_ResumeTimeout_1 for details.
  EOD
  type        = number
  default     = 300

  validation {
    condition     = var.resume_timeout == null ? true : var.resume_timeout > 0
    error_message = "Value must be > 0."
  }
}

variable "suspend_time" {
  description = <<-EOD
    Nodes which remain idle or down for this number of seconds will be placed into power save mode by SuspendProgram.
    This sets 'SuspendTime' in partition_conf.
    See https://slurm.schedmd.com/slurm.conf.html#OPT_SuspendTime_1 for details.
    NOTE: use value -1 to exclude partition from suspend.
  EOD
  type        = number
  default     = 300

  validation {
    condition     = var.suspend_time >= -1
    error_message = "Value must be >= -1."
  }
}

variable "suspend_timeout" {
  description = <<-EOD
    Maximum time permitted (in seconds) between when a node suspend request is issued and when the node is shutdown.
    If null is given, then a smart default will be chosen depending on nodesets in partition.
    This sets 'SuspendTimeout' in partition_conf.
    See https://slurm.schedmd.com/slurm.conf.html#OPT_SuspendTimeout_1 for details.
  EOD
  type        = number
  default     = null

  validation {
    condition     = var.suspend_timeout == null ? true : var.suspend_timeout > 0
    error_message = "Value must be > 0."
  }
}


variable "nodeset" {
  description = <<-EOT
    A list of nodesets associated with this partition. 
    Do not specifi manually, use the nodeset module instead.
    EOT
  // TODO: use any ?
  type = list(object({
    node_count_static      = optional(number, 0)
    node_count_dynamic_max = optional(number, 1)
    node_conf              = optional(map(string), {})
    nodeset_name           = string
    additional_disks = optional(list(object({
      disk_name    = optional(string)
      device_name  = optional(string)
      disk_size_gb = optional(number)
      disk_type    = optional(string)
      disk_labels  = optional(map(string), {})
      auto_delete  = optional(bool, true)
      boot         = optional(bool, false)
    })), [])
    bandwidth_tier         = optional(string, "platform_default")
    can_ip_forward         = optional(bool, false)
    disable_smt            = optional(bool, false)
    disk_auto_delete       = optional(bool, true)
    disk_labels            = optional(map(string), {})
    disk_size_gb           = optional(number)
    disk_type              = optional(string)
    enable_confidential_vm = optional(bool, false)
    enable_placement       = optional(bool, false)
    enable_public_ip       = optional(bool, false)
    enable_oslogin         = optional(bool, true)
    enable_shielded_vm     = optional(bool, false)
    gpu = optional(object({
      count = number
      type  = string
    }))
    instance_template   = optional(string)
    labels              = optional(map(string), {})
    machine_type        = optional(string)
    metadata            = optional(map(string), {})
    min_cpu_platform    = optional(string)
    network_tier        = optional(string, "STANDARD")
    on_host_maintenance = optional(string)
    preemptible         = optional(bool, false)
    region              = optional(string)
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
    subnetwork_project   = optional(string)
    subnetwork           = optional(string)
    spot                 = optional(bool, false)
    tags                 = optional(list(string), [])
    termination_action   = optional(string)
    zones                = optional(list(string), [])
    zone_target_shape    = optional(string, "ANY_SINGLE_ZONE")
  }))
  default = []

  validation {
    condition     = length(distinct([for x in var.nodeset : x.nodeset_name])) == length(var.nodeset)
    error_message = "All nodesets must have a unique name."
  }
}