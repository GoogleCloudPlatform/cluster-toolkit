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
  default     = null

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
    DO NOT specifi manually, use the nodeset module instead.
    EOT
  type        = list(any) # TODO: add note about source of truth
  default     = []

  validation {
    condition     = length(distinct([for ns in var.nodeset : ns.name])) == length(var.nodeset)
    error_message = "All nodesets must have a unique name."
  }
}