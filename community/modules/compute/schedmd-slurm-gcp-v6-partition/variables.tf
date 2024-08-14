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

variable "partition_name" {
  description = "The name of the slurm partition."
  type        = string

  validation {
    condition     = can(regex("^[a-z](?:[a-z0-9]*)$", var.partition_name))
    error_message = "Variable 'partition_name' must be a match of regex '^[a-z](?:[a-z0-9]*)$'."
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

variable "is_default" {
  description = <<-EOD
    Sets this partition as the default partition by updating the partition_conf.
    If "Default" is already set in partition_conf, this variable will have no effect.
    EOD
  type        = bool
  default     = false
}

variable "exclusive" {
  description = <<-EOD
    Exclusive job access to nodes. When set to true nodes execute single job and are deleted
    after job exits. If set to false, multiple jobs can be scheduled on one node.
    EOD
  type        = bool
  default     = true
}

variable "nodeset" {
  description = <<-EOD
  A list of nodesets.
  For type definition see community/modules/scheduler/schedmd-slurm-gcp-v6-controller/variables.tf::nodeset
  EOD
  type        = list(any)
  default     = []

  validation {
    condition     = length(distinct(var.nodeset[*].nodeset_name)) == length(var.nodeset)
    error_message = "All nodesets must have a unique name."
  }
}

variable "nodeset_tpu" {
  description = "Define TPU nodesets, as a list."
  type = list(object({
    node_count_static      = optional(number, 0)
    node_count_dynamic_max = optional(number, 5)
    nodeset_name           = string
    enable_public_ip       = optional(bool, false)
    node_type              = string
    accelerator_config = optional(object({
      topology = string
      version  = string
      }), {
      topology = ""
      version  = ""
    })
    tf_version   = string
    preemptible  = optional(bool, false)
    preserve_tpu = optional(bool, false)
    zone         = string
    data_disks   = optional(list(string), [])
    docker_image = optional(string, "")
    network_storage = optional(list(object({
      server_ip     = string
      remote_mount  = string
      local_mount   = string
      fs_type       = string
      mount_options = string
    })), [])
    subnetwork = string
    service_account = optional(object({
      email  = optional(string)
      scopes = optional(list(string), ["https://www.googleapis.com/auth/cloud-platform"])
    }))
    project_id = string
    reserved   = optional(string, false)
  }))
  default = []

  validation {
    condition     = length(distinct([for x in var.nodeset_tpu : x.nodeset_name])) == length(var.nodeset_tpu)
    error_message = "All TPU nodesets must have a unique name."
  }
}

variable "nodeset_dyn" {
  description = "Defines dynamic nodesets, as a list."
  type = list(object({
    nodeset_name    = string
    nodeset_feature = string
  }))
  default = []

  validation {
    condition     = length(distinct([for x in var.nodeset_dyn : x.nodeset_name])) == length(var.nodeset_dyn)
    error_message = "All dynamic nodesets must have a unique name."
  }
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
    NOTE 2: if `var.exclusive` is set to true (default), nodes are deleted immediately after job finishes.
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


# tflint-ignore: terraform_unused_declarations
variable "network_storage" {
  description = "DEPRECATED"
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
  validation {
    condition     = length(var.network_storage) == 0
    error_message = <<-EOD
      network_storage in partition module is deprecated and should not be set.
      To add network storage to compute nodes, use network_storage of nodeset module instead.
    EOD
  }
}
