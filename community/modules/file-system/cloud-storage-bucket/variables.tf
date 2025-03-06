/**
 * Copyright 2023 Google LLC
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
  description = "ID of project in which GCS bucket will be created."
  type        = string
}

variable "deployment_name" {
  description = "Name of the HPC deployment; used as part of name of the GCS bucket."
  type        = string
}

variable "region" {
  description = "The region to deploy to"
  type        = string
}

variable "labels" {
  description = "Labels to add to the GCS bucket. Key-value pairs."
  type        = map(string)
}

variable "local_mount" {
  description = "The mount point where the contents of the device may be accessed after mounting."
  type        = string
  default     = "/mnt"
}

variable "mount_options" {
  description = "Mount options to be put in fstab. Note: `implicit_dirs` makes it easier to work with objects added by other tools, but there is a performance impact. See: [more information](https://github.com/GoogleCloudPlatform/gcsfuse/blob/master/docs/semantics.md#implicit-directories)"
  type        = string
  default     = "defaults,_netdev,implicit_dirs"
}

variable "name_prefix" {
  description = "Name Prefix."
  type        = string
  default     = null
}

variable "use_deployment_name_in_bucket_name" {
  description = "If true, the deployment name will be included as part of the bucket name. This helps prevent naming clashes across multiple deployments."
  type        = bool
  default     = true
}

variable "random_suffix" {
  description = "If true, a random id will be appended to the suffix of the bucket name."
  type        = bool
  default     = false
}

variable "force_destroy" {
  description = "If true will destroy bucket with all objects stored within."
  type        = bool
  default     = false
}

variable "viewers" {
  description = "A list of additional accounts that can read packages from this bucket"
  type        = set(string)
  default     = []

  validation {
    error_message = "All bucket viewers must be in IAM style: user:user@example.com, serviceAccount:sa@example.com, or group:group@example.com."
    condition = alltrue([
      for viewer in var.viewers : length(regexall("^(user|serviceAccount|group):", viewer)) > 0
    ])
  }
}

variable "enable_hierarchical_namespace" {
  description = "If true, enables hierarchical namespace for the bucket. This option must be configured during the initial creation of the bucket."
  type        = bool
  default     = false
}
