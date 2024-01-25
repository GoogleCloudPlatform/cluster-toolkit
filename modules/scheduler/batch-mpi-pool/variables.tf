/**
 * Copyright 2024 Google LLC
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

variable "project_id" {
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "region" {
  description = "The region in which to create the node pool"
  type        = string
}

variable "pool_size" {
  description = "Number of VMs to add to the node pool"
  type        = number
  default     = 4
}

variable "pool_duration" {
  description = "Maximum idle time for the pool, after which it will be automatically deprovisioned"
  type        = string
  default     = "1h"
}

variable "machine_type" {
  description = "Machine type for VMs in the pool"
  type        = string
  default     = "c2-standard-60"
}

variable "boot_image" {
  description = "Boot image for the VMs in the pool"
  type        = string
  default     = "batch-hpc-centos"
}

variable "nfs_share" {
  description = "An NFS share (optional) to be mounted by each node in the pool"
  type = object({
    server_ip            = string
    remote_path          = string
    mount_path           = string
  })
}

variable "deployment_name" {
  description = "Name of the deployment, used for the pool name"
  type        = string
}

variable "gcloud_version" {
  description = "The version of the gcloud cli being used. Used for output instructions. Valid inputs are `\"alpha\"`, `\"beta\"` and \"\" (empty string for default version)"
  type        = string
  default     = "alpha"

  validation {
    condition     = contains(["alpha", "beta", ""], var.gcloud_version)
    error_message = "Allowed values for gcloud_version are 'alpha', 'beta', or '' (empty string)."
  }
}
