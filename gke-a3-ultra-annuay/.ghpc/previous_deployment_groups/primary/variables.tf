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

variable "a3ultra_node_pool_disk_size_gb" {
  description = "Toolkit deployment variable: a3ultra_node_pool_disk_size_gb"
  type        = number
}

variable "authorized_cidr" {
  description = "Toolkit deployment variable: authorized_cidr"
  type        = string
}

variable "deployment_name" {
  description = "Toolkit deployment variable: deployment_name"
  type        = string
}

variable "extended_reservation" {
  description = "Toolkit deployment variable: extended_reservation"
  type        = string
}

variable "labels" {
  description = "Toolkit deployment variable: labels"
  type        = any
}

variable "mglru_disable_path" {
  description = "Toolkit deployment variable: mglru_disable_path"
  type        = string
}

variable "mtu_size" {
  description = "Toolkit deployment variable: mtu_size"
  type        = number
}

variable "nccl_installer_path" {
  description = "Toolkit deployment variable: nccl_installer_path"
  type        = string
}

variable "project_id" {
  description = "Toolkit deployment variable: project_id"
  type        = string
}

variable "region" {
  description = "Toolkit deployment variable: region"
  type        = string
}

variable "static_node_count" {
  description = "Toolkit deployment variable: static_node_count"
  type        = number
}

variable "system_node_pool_disk_size_gb" {
  description = "Toolkit deployment variable: system_node_pool_disk_size_gb"
  type        = number
}

variable "zone" {
  description = "Toolkit deployment variable: zone"
  type        = string
}
