/**
 * Copyright (C) SchedMD LLC.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

variable "nodeset_name" {
  description = "Name of Slurm nodeset."
  type        = string

  validation {
    condition     = can(regex("^[a-z](?:[a-z0-9]{0,14})$", var.nodeset_name))
    error_message = "Variable 'nodeset_name' must be a match of regex '^[a-z](?:[a-z0-9]{0,14})$'."
  }
}

variable "node_type" {
  description = "Specify a node type to base the vm configuration upon it. Not needed if you use accelerator_config"
  type        = string
  default     = null
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
    condition     = var.accelerator_config.version == "" ? true : contains(["V2", "V3", "V4"], upper(var.accelerator_config.version))
    error_message = "accelerator_config.version must be one of [\"V2\", \"V3\", \"V4\"]"
  }
  validation {
    condition     = var.accelerator_config.topology == "" ? true : can(regex("^[1-9]x[1-9](x[1-9])?$", var.accelerator_config.topology))
    error_message = "accelerator_config.topology must be a valid topology, like 2x2 4x4x4 4x2x4 etc..."
  }
}

variable "docker_image" {
  description = "The gcp container registry id docker image to use in the TPU vms, it defaults to gcr.io/schedmd-slurm-public/tpu:slurm-gcp-6-8-tf-<var.tf_version>"
  type        = string
  default     = ""
}

variable "tf_version" {
  description = "Nodeset Tensorflow version, see https://cloud.google.com/tpu/docs/supported-tpu-configurations#tpu_vm for details."
  type        = string
}

variable "zone" {
  description = "Nodes will only be created in this zone. Check https://cloud.google.com/tpu/docs/regions-zones to get zones with TPU-vm in it."
  type        = string

  validation {
    condition     = can(coalesce(var.zone))
    error_message = "Zone cannot be null or empty."
  }
}

variable "preemptible" {
  description = "Specify whether TPU-vms in this nodeset are preemtible, see https://cloud.google.com/tpu/docs/preemptible for details."
  type        = bool
  default     = false
}

variable "reserved" {
  description = "Specify whether TPU-vms in this nodeset are created under a reservation."
  type        = bool
  default     = false
}

variable "preserve_tpu" {
  description = "Specify whether TPU-vms will get preserve on suspend, if set to true, on suspend vm is stopped, on false it gets deleted"
  type        = bool
  default     = true
}

variable "node_count_static" {
  description = "Number of nodes to be statically created."
  type        = number
  default     = 0

  validation {
    condition     = var.node_count_static >= 0
    error_message = "Value must be >= 0."
  }
}

variable "node_count_dynamic_max" {
  description = "Maximum number of nodes allowed in this partition to be created dynamically."
  type        = number
  default     = 0

  validation {
    condition     = var.node_count_dynamic_max >= 0
    error_message = "Value must be >= 0."
  }
}

variable "enable_public_ip" {
  description = "Enables IP address to access the Internet."
  type        = bool
  default     = false
}

variable "data_disks" {
  type        = list(string)
  description = "The data disks to include in the TPU node"
  default     = []
}

variable "subnetwork" {
  description = "The name of the subnetwork to attach the TPU-vm of this nodeset to."
  type        = string
}

variable "service_account" {
  type = object({
    email  = string
    scopes = set(string)
  })
  description = <<EOD
Service account to attach to the TPU-vm.
If none is given, the default service account and scopes will be used.
EOD
  default     = null
}

variable "project_id" {
  type        = string
  description = "Project ID to create resources in."
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
