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

variable "a4_node_pool_disk_size_gb" {
  description = "Toolkit deployment variable: a4_node_pool_disk_size_gb"
  type        = number
}

variable "accelerator_type" {
  description = "Toolkit deployment variable: accelerator_type"
  type        = string
}

variable "authorized_cidr" {
  description = "Toolkit deployment variable: authorized_cidr"
  type        = string
}

variable "chs_cronjob_rendered_path" {
  description = "Toolkit deployment variable: chs_cronjob_rendered_path"
  type        = string
}

variable "chs_output_bucket_name" {
  description = "Toolkit deployment variable: chs_output_bucket_name"
  type        = string
}

variable "chs_pvc_claim_name" {
  description = "Toolkit deployment variable: chs_pvc_claim_name"
  type        = string
}

variable "chs_pvc_rendered_path" {
  description = "Toolkit deployment variable: chs_pvc_rendered_path"
  type        = string
}

variable "deployment_name" {
  description = "Toolkit deployment variable: deployment_name"
  type        = string
}

variable "enable_periodic_health_checks" {
  description = "Toolkit deployment variable: enable_periodic_health_checks"
  type        = bool
}

variable "gib_installer_path" {
  description = "Toolkit deployment variable: gib_installer_path"
  type        = string
}

variable "health_check_schedule" {
  description = "Toolkit deployment variable: health_check_schedule"
  type        = string
}

variable "kueue_configuration_path" {
  description = "Toolkit deployment variable: kueue_configuration_path"
  type        = string
}

variable "labels" {
  description = "Toolkit deployment variable: labels"
  type        = any
}

variable "permissions_file_staged_path" {
  description = "Toolkit deployment variable: permissions_file_staged_path"
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

variable "version_prefix" {
  description = "Toolkit deployment variable: version_prefix"
  type        = string
}

variable "zone" {
  description = "Toolkit deployment variable: zone"
  type        = string
}
