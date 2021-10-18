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

variable "depends" {
  description = "Allows to add explicit dependencies"
  default     = null
  type        = list(any)
}

variable "deployment_name" {
  description = "Name of the deployment, used to name the cluster"
  type        = string
}

variable "manager_node" {
  description = "Name of the Omnia manager node"
  type        = string
}

variable "zone" {
  description = "The GCP zone where the Omnia cluster is running"
  type        = string
}

variable "project_id" {
  description = "Project in which the Omnia cluster has been created"
  type        = string
}
