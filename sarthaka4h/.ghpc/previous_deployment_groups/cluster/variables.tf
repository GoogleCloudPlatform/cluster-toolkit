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

variable "a4h_cluster_size" {
  description = "Toolkit deployment variable: a4h_cluster_size"
  type        = number
}

variable "a4h_dws_flex_enabled" {
  description = "Toolkit deployment variable: a4h_dws_flex_enabled"
  type        = bool
}

variable "a4h_enable_spot_vm" {
  description = "Toolkit deployment variable: a4h_enable_spot_vm"
  type        = bool
}

variable "a4h_reservation_name" {
  description = "Toolkit deployment variable: a4h_reservation_name"
  type        = string
}

variable "benchmark_dir" {
  description = "Toolkit deployment variable: benchmark_dir"
  type        = string
}

variable "client_install_runner_gcs_checkpoints" {
  description = "Automatically generated input from previous groups (gcluster import-inputs --help)"
  type        = any
}

variable "client_install_runner_gcs_model_serving" {
  description = "Automatically generated input from previous groups (gcluster import-inputs --help)"
  type        = any
}

variable "client_install_runner_gcs_training_data" {
  description = "Automatically generated input from previous groups (gcluster import-inputs --help)"
  type        = any
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

variable "libnccl_version" {
  description = "Toolkit deployment variable: libnccl_version"
  type        = string
}

variable "local_ssd_mountpoint" {
  description = "Toolkit deployment variable: local_ssd_mountpoint"
  type        = string
}

variable "mount_runner_gcs_checkpoints" {
  description = "Automatically generated input from previous groups (gcluster import-inputs --help)"
  type        = any
}

variable "mount_runner_gcs_model_serving" {
  description = "Automatically generated input from previous groups (gcluster import-inputs --help)"
  type        = any
}

variable "mount_runner_gcs_training_data" {
  description = "Automatically generated input from previous groups (gcluster import-inputs --help)"
  type        = any
}

variable "nccl_gib_version" {
  description = "Toolkit deployment variable: nccl_gib_version"
  type        = string
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

variable "subnetwork_interfaces_a4high-slurm-rdma-net" {
  description = "Automatically generated input from previous groups (gcluster import-inputs --help)"
  type        = any
}

variable "subnetwork_self_link_a4high-slurm-net-0" {
  description = "Automatically generated input from previous groups (gcluster import-inputs --help)"
  type        = any
}

variable "subnetwork_self_link_a4high-slurm-net-1" {
  description = "Automatically generated input from previous groups (gcluster import-inputs --help)"
  type        = any
}

variable "zone" {
  description = "Toolkit deployment variable: zone"
  type        = string
}
