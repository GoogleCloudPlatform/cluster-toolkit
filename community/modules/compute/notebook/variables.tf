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

variable "project_id" {
  description = "ID of project in which GCS bucket will be created."
  type        = string
}

variable "deployment_name" {
  description = "Name of the HPC deployment; used as part of name of the GCS bucket."
  type        = string
}

variable "create_tutorial" {
  description = "Which tutorial files should be created."
  type        = string
  default = ""
}


variable "zone" {
  description = "The zone to deploy to"
  type        = string
}

variable "machine_type" {
  description = "The machine type to employ"
  type        = string
}

variable "labels" {
  description = "Labels to add to the resource Key-value pairs."
  type        = map(string)
}

variable "image_project_id" {
  description = "Project Id for Image hosting"
  type        = string
  default     = ""
}

variable "image_family" {
  description = "Image Family"
  type        = string
  default     = ""
}

variable "name_prefix" {
  description = "Name Prefix."
  type        = string
  default     = null
}
variable "gcs_bucket_path" {
  description = "Bucket name"
  type        = string
  default     = null
}

variable "mount_runner" {
  description = "mount content "
  type        = any
}

variable "topic_id" {
  description = "Pubsub Topic Name"
  type        = string
}

variable "topic_schema" {
  description = "Pubsub Topic schema"
  type        = string
}

variable "dataset_id" {
  description = "Bigquery dataset id"
  type        = string
}

variable "table_id" {
  description = "Bigquery table id"
  type        = string
}

variable "region" {
  description = "Region to run project"
  type        = string
}