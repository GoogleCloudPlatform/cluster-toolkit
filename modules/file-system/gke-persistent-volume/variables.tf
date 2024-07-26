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

variable "cluster_id" {
  description = "An identifier for the GKE cluster in the format `projects/{{project}}/locations/{{location}}/clusters/{{cluster}}`"
  type        = string
}

variable "network_storage" {
  description = "Network attached storage mount to be configured."
  type = object({
    server_ip             = string,
    remote_mount          = string,
    local_mount           = string,
    fs_type               = string,
    mount_options         = string,
    client_install_runner = map(string)
    mount_runner          = map(string)
  })
}

variable "filestore_id" {
  description = "An identifier for a filestore with the format `projects/{{project}}/locations/{{location}}/instances/{{name}}`."
  type        = string
  default     = null
  validation {
    condition = (
      var.filestore_id == null ||
      try(length(split("/", var.filestore_id)), 0) == 6
    )
    error_message = "filestore_id must be in the format of 'projects/{{project}}/locations/{{location}}/instances/{{name}}'."
  }
}

variable "gcs_bucket_name" {
  description = "The gcs bucket to be used with the persistent volume."
  type        = string
  default     = null
}

variable "capacity_gb" {
  description = "The storage capacity with which to create the persistent volume."
  type        = number
}

variable "labels" {
  description = "GCE resource labels to be applied to resources. Key-value pairs."
  type        = map(string)
}
