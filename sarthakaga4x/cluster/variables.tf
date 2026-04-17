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

variable "a4x_cluster_size" {
  description = "Toolkit deployment variable: a4x_cluster_size"
  type        = number
}

variable "a4x_reservation_name" {
  description = "Toolkit deployment variable: a4x_reservation_name"
  type        = string
}

variable "benchmark_dir" {
  description = "Toolkit deployment variable: benchmark_dir"
  type        = string
}

variable "deployment_name" {
  description = "Toolkit deployment variable: deployment_name"
  type        = string
}

variable "disk_size_gb" {
  description = "Toolkit deployment variable: disk_size_gb"
  type        = number
}

variable "instance_image" {
  description = "Toolkit deployment variable: instance_image"
  type        = any
}

variable "labels" {
  description = "Toolkit deployment variable: labels"
  type        = any
}

variable "network_storage_gcs_bucket" {
  description = "Automatically generated input from previous groups (gcluster import-inputs --help)"
  type        = any
}

variable "network_storage_homefs" {
  description = "Automatically generated input from previous groups (gcluster import-inputs --help)"
  type        = any
}

variable "project_id" {
  description = "Toolkit deployment variable: project_id"
  type        = string
}

variable "region" {
  description = "Toolkit deployment variable: region"
  type        = string
}

variable "startup_script_a4x_startup" {
  description = "Automatically generated input from previous groups (gcluster import-inputs --help)"
  type        = any
}

variable "subnetwork_interfaces_a4x-slurm-rdma-net" {
  description = "Automatically generated input from previous groups (gcluster import-inputs --help)"
  type        = any
}

variable "subnetwork_self_link_a4x-slurm-net-0" {
  description = "Automatically generated input from previous groups (gcluster import-inputs --help)"
  type        = any
}

variable "subnetwork_self_link_a4x-slurm-net-1" {
  description = "Automatically generated input from previous groups (gcluster import-inputs --help)"
  type        = any
}

variable "zone" {
  description = "Toolkit deployment variable: zone"
  type        = string
}
