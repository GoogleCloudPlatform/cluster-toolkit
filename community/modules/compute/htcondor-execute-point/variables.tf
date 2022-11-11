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

variable "project_id" {
  description = "Project in which the HTCondor execute points will be created"
  type        = string
}

variable "region" {
  description = "The region in which HTCondor execute points will be created"
  type        = string
}

variable "zone" {
  description = "The default zone in which resources will be created"
  type        = string
}

variable "deployment_name" {
  description = "HPC Toolkit deployment name. HTCondor cloud resource names will include this value."
  type        = string
}

variable "labels" {
  description = "Labels to add to HTConodr execute points"
  type        = map(string)
}

variable "machine_type" {
  description = "Machine type to use for HTCondor execute points"
  type        = string
  default     = "n2-standard-4"
}

variable "startup_script" {
  description = "Startup script to run at boot-time for HTCondor execute points"
  type        = string
  default     = null
}

variable "network_storage" {
  description = "An array of network attached storage mounts to be configured"
  type = list(object({
    server_ip     = string,
    remote_mount  = string,
    local_mount   = string,
    fs_type       = string,
    mount_options = string
  }))
  default = []
}

variable "image" {
  description = "HTCondor execute point VM image"
  type = object({
    family  = string,
    project = string
  })
  default = {
    family  = "hpc-centos-7"
    project = "cloud-hpc-image-public"
  }
}

variable "service_account" {
  description = "Service account to attach to HTCondor execute points"
  type = object({
    email  = string,
    scopes = set(string)
  })
  default = {
    email = null
    scopes = [
      "https://www.googleapis.com/auth/cloud-platform",
    ]
  }
}

variable "network_self_link" {
  description = "The self link of the network HTCondor execute points will join"
  type        = string
  default     = "default"
}

variable "subnetwork_self_link" {
  description = "The self link of the subnetwork HTCondor execute points will join"
  type        = string
  default     = null
}

variable "target_size" {
  description = "Initial size of the HTCondor execute point pool; set to null (default) to avoid Terraform management of size."
  type        = number
  default     = null
}

variable "max_size" {
  description = "Maximum size of the HTCondor execute point pool; set to constrain cost run-away."
  type        = number
  default     = 100
}

variable "metadata" {
  description = "Metadata to add to HTCondor execute points"
  type        = map(string)
  default     = {}
}

# this default is deliberately the opposite of vm-instance because of observed
# issues running HTCondor docker universe jobs with OS Login enabled and running
# jobs as a user with uid>2^31; these uids occur when users outside the GCP
# organization login to a VM and OS Login is enabled.
variable "enable_oslogin" {
  description = "Enable or Disable OS Login with \"ENABLE\" or \"DISABLE\". Set to \"INHERIT\" to inherit project OS Login setting."
  type        = string
  default     = "DISABLE"
}
