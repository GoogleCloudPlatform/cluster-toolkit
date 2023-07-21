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
  description = "Project in which HTCondor pool will be created"
  type        = string
}

variable "deployment_name" {
  description = "HPC Toolkit deployment name. HTCondor cloud resource names will include this value."
  type        = string
}

variable "labels" {
  description = "Labels to add to resources. List key, value pairs."
  type        = map(string)
}

variable "region" {
  description = "Default region for creating resources"
  type        = string
}

variable "subnetwork_self_link" {
  description = "The self link of the subnetwork in which Central Managers will be placed."
  type        = string
}

variable "access_point_roles" {
  description = "Project-wide roles for HTCondor Access Point service account"
  type        = list(string)
  default = [
    "roles/compute.instanceAdmin",
    "roles/monitoring.metricWriter",
    "roles/logging.logWriter",
    "roles/storage.objectViewer",
  ]
}

variable "central_manager_roles" {
  description = "Project-wide roles for HTCondor Central Manager service account"
  type        = list(string)
  default = [
    "roles/monitoring.metricWriter",
    "roles/logging.logWriter",
    "roles/storage.objectViewer",
  ]
}

variable "execute_point_roles" {
  description = "Project-wide roles for HTCondor Execute Point service account"
  type        = list(string)
  default = [
    "roles/monitoring.metricWriter",
    "roles/logging.logWriter",
    "roles/storage.objectViewer",
  ]
}
