/**
 * Copyright (C) Google LLC.
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

variable "project_id" {
  type        = string
  description = "Project ID"
}

variable "slurm_cluster_name" {
  type        = string
  description = "Name of the Slurm cluster"
}

variable "enable_cleanup_compute" {
  description = <<EOD
Enables automatic cleanup of TPU nodes managed by this module, when cluster is destroyed.

*WARNING*: Toggling this off will impact the running workload.
Deployed TPU nodes will be destroyed.
EOD
  type        = bool
}

variable "universe_domain" {
  description = "Domain address for alternate API universe"
  type        = string
}

variable "endpoint_versions" {
  description = "Version of the API to use (The compute service is the only API currently supported)"
  type = object({
    compute = string
  })
}

variable "gcloud_path_override" {
  description = "Directory of the gcloud executable to be used during cleanup"
  type        = string
}

variable "nodeset" {
  description = "Nodeset to cleanup"
  type = object({
    nodeset_name = string
    zone         = string
  })
}
