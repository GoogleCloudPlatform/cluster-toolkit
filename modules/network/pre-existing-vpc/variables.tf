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

variable "project_id" {
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "deployment_name" {
  description = <<EOT
    The name of the current deployment; default values for network_name and
    subnetwork_name will be derived in manner identical to Toolkit VPC module.
    EOT
  type        = string
}

variable "use_default_network" {
  description = <<EOT
    If no values for network_name or subnetwork_name are supplied, use 'default'
    GCP network rather than Toolkit defaults based on deployment_name
    EOT
  type        = bool
  default     = true
}

variable "network_name" {
  description = "The name of the network whose attributes will be found"
  type        = string
  default     = null
}

variable "subnetwork_name" {
  description = "The name of the subnetwork to returned, will use network name if null."
  type        = string
  default     = null
}

variable "region" {
  description = "The region where Cloud NAT and Cloud Router will be configured"
  type        = string
}
