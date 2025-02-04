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

variable "node_count_static" {
  description = <<-EOD
    Number of worker nodes to be statically created. 
    For larger TPU machines, there are multiple worker nodes required per machine (1 for every 8 cores).
    See https://cloud.google.com/tpu/docs/v4#large-topologies, for more information about these machine types.
    EOD
  type        = number
  default     = 0
}

variable "node_count_dynamic_max" {
  description = <<-EOD
    Maximum number of auto-scaling worker nodes allowed in this partition. 
    For larger TPU machines, there are multiple worker nodes required per machine (1 for every 8 cores).
    See https://cloud.google.com/tpu/docs/v4#large-topologies, for more information about these machine types.
    EOD
  type        = number
  default     = 0
}

variable "name" {
  description = <<-EOD
    Name of the nodeset. Automatically populated by the module id if not set. 
    If setting manually, ensure a unique value across all nodesets.
    EOD
  type        = string
}

variable "enable_public_ips" {
  description = "If set to true. The node group VMs will have a random public IP assigned to it. Ignored if access_config is set."
  type        = bool
  default     = false
}

variable "disable_public_ips" { # tflint-ignore: terraform_unused_declarations
  description = "DEPRECATED: Use `enable_public_ips` instead."
  type        = bool
  default     = null
  validation {
    condition     = var.disable_public_ips == null
    error_message = "DEPRECATED: Use `enable_public_ips` instead."
  }
}

variable "node_type" {
  description = "Specify a node type to base the vm configuration upon it."
  type        = string
  default     = ""
}

variable "accelerator_config" {
  description = "Nodeset accelerator config, see https://cloud.google.com/tpu/docs/supported-tpu-configurations for details."
  type = object({
    topology = string
    version  = string
  })
  default = {
    topology = ""
    version  = ""
  }
  validation {
    condition     = var.accelerator_config.version == "" ? true : contains(["V2", "V3", "V4"], var.accelerator_config.version)
    error_message = "accelerator_config.version must be one of [\"V2\", \"V3\", \"V4\"]"
  }
  validation {
    condition     = var.accelerator_config.topology == "" ? true : can(regex("^[1-9]x[1-9](x[1-9])?$", var.accelerator_config.topology))
    error_message = "accelerator_config.topology must be a valid topology, like 2x2 4x4x4 4x2x4 etc..."
  }
}

variable "tf_version" {
  description = "Nodeset Tensorflow version, see https://cloud.google.com/tpu/docs/supported-tpu-configurations#tpu_vm for details."
  type        = string
  default     = "2.14.0"
}

variable "preemptible" {
  description = "Should use preemptibles to burst."
  type        = bool
  default     = false
}

variable "preserve_tpu" {
  description = "Specify whether TPU-vms will get preserve on suspend, if set to true, on suspend vm is stopped, on false it gets deleted"
  type        = bool
  default     = false
}

variable "zone" {
  description = "Zone in which to create compute VMs. TPU partitions can only specify a single zone."
  type        = string
}

variable "data_disks" {
  description = "The data disks to include in the TPU node"
  type        = list(string)
  default     = []
}

variable "docker_image" {
  description = "The gcp container registry id docker image to use in the TPU vms, it defaults to gcr.io/schedmd-slurm-public/tpu:slurm-gcp-6-8-tf-<var.tf_version>"
  type        = string
  default     = null
}

variable "subnetwork_self_link" {
  type        = string
  description = "The name of the subnetwork to attach the TPU-vm of this nodeset to."
}

variable "service_account_email" {
  description = "Service account e-mail address to attach to the TPU-vm."
  type        = string
  default     = null
}

variable "service_account_scopes" {
  description = "Scopes to attach to the TPU-vm."
  type        = set(string)
  default     = ["https://www.googleapis.com/auth/cloud-platform"]
}

variable "service_account" { # tflint-ignore: terraform_unused_declarations
  description = "DEPRECATED: Use `service_account_email` and `service_account_scopes` instead."
  type = object({
    email  = string
    scopes = set(string)
  })
  default = null
  validation {
    condition     = var.service_account == null
    error_message = "DEPRECATED: Use `service_account_email` and `service_account_scopes` instead."
  }
}

variable "project_id" {
  type        = string
  description = "Project ID to create resources in."
}

variable "reserved" {
  description = "Specify whether TPU-vms in this nodeset are created under a reservation."
  type        = bool
  default     = false
}

variable "network_storage" {
  description = "An array of network attached storage mounts to be configured on nodes."
  type = list(object({
    server_ip     = string,
    remote_mount  = string,
    local_mount   = string,
    fs_type       = string,
    mount_options = string,
  }))
  default = []
}
