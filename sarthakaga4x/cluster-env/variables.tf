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

variable "deployment_name" {
  description = "Toolkit deployment variable: deployment_name"
  type        = string
}

variable "filestore_ip_range" {
  description = "Toolkit deployment variable: filestore_ip_range"
  type        = string
}

variable "labels" {
  description = "Toolkit deployment variable: labels"
  type        = any
}

variable "local_ssd_mountpoint" {
  description = "Toolkit deployment variable: local_ssd_mountpoint"
  type        = string
}

variable "nccl_plugin_version" {
  description = "Toolkit deployment variable: nccl_plugin_version"
  type        = string
}

variable "net0_range" {
  description = "Toolkit deployment variable: net0_range"
  type        = string
}

variable "net1_range" {
  description = "Toolkit deployment variable: net1_range"
  type        = string
}

variable "project_id" {
  description = "Toolkit deployment variable: project_id"
  type        = string
}

variable "rdma_net_range" {
  description = "Toolkit deployment variable: rdma_net_range"
  type        = string
}

variable "region" {
  description = "Toolkit deployment variable: region"
  type        = string
}

variable "zone" {
  description = "Toolkit deployment variable: zone"
  type        = string
}
