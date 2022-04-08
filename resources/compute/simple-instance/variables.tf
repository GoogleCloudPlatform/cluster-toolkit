/**
 * Copyright 2021 Google LLC
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

variable "instance_count" {
  description = "Number of instances"
  type        = number
  default     = 1
}

variable "instance_image" {
  description = "Instance Image"
  type = object({
    family  = string,
    project = string
  })
  default = {
    family  = "hpc-centos-7"
    project = "cloud-hpc-image-public"
  }
}

variable "disk_size_gb" {
  description = "Size of disk for instances."
  type        = number
  default     = 200
}

variable "disk_type" {
  description = "Disk type for instances."
  type        = string
  default     = "pd-standard"
}

variable "name_prefix" {
  description = "Name Prefix"
  type        = string
  default     = null
}

variable "disable_public_ips" {
  description = "If set to true, instances will not have public IPs"
  type        = bool
  default     = false
}

variable "machine_type" {
  description = "Machine type to use for the instance creation"
  type        = string
  default     = "c2-standard-60"
}

variable "network_storage" {
  description = "An array of network attached storage mounts to be configured."
  type = list(object({
    server_ip     = string,
    remote_mount  = string,
    local_mount   = string,
    fs_type       = string,
    mount_options = string
  }))
  default = []
}

variable "deployment_name" {
  description = "Name of the deployment, used to name the cluster"
  type        = string
}

variable "labels" {
  description = "Labels to add to the instances. List key, value pairs."
  type        = any
}

variable "service_account" {
  description = "Service account to attach to the instance. See https://www.terraform.io/docs/providers/google/r/compute_instance_template.html#service_account."
  type = object({
    email  = string,
    scopes = set(string)
  })
  default = {
    email = null
    scopes = ["https://www.googleapis.com/auth/devstorage.read_only",
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring.write",
      "https://www.googleapis.com/auth/servicecontrol",
      "https://www.googleapis.com/auth/service.management.readonly",
    "https://www.googleapis.com/auth/trace.append"]
  }
}

variable "network_self_link" {
  description = "The self link of the network to attach the VM."
  type        = string
  default     = "default"
}

variable "subnetwork_self_link" {
  description = "The self link of the subnetwork to attach the VM."
  type        = string
  default     = null
}

variable "zone" {
  description = "Compute Platform zone"
  type        = string
}

variable "metadata" {
  description = "Metadata, provided as a map"
  type        = map(string)
  default     = {}
}

variable "startup_script" {
  description = "Startup script used on the instance"
  type        = string
  default     = null
}

variable "guest_accelerator" {
  description = "List of the type and count of accelerator cards attached to the instance."
  type = list(object({
    type  = string,
    count = number
  }))
  default = []
}

variable "on_host_maintenance" {
  description = "Describes maintenance behavior for the instance."
  type        = string
  default     = "MIGRATE"
  validation {
    condition     = contains(["MIGRATE", "TERMINATE"], var.on_host_maintenance)
    error_message = "The on_host_maintenance must be set to MIGRATE or TERMINATE."
  }
}

variable "advanced_networking" {
  description = <<EOT
  Enables advanced networking features on the simple instance. 
  Using the `tier_1_enabled` setting will enable both gVNIC and TIER_1 higher bandwidth networking. 
  Note that TIER_1 only works with specific machine families & shapes and must be using an image that supports gVNIC.
  EOT
  type        = string
  default     = "not_enabled"

  validation {
    condition     = contains(["not_enabled", "gvnic_enabled", "tier_1_enabled"], var.advanced_networking)
    error_message = "Allowed values for advanced_networking are 'not_enabled', 'gvnic_enabled', or  'tier_1_enabled'."
  }
}
