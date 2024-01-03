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
  description = "ID of project in which the notebook will be created."
  type        = string
}

variable "deployment_name" {
  description = "Name of the HPC deployment; used as part of name of the notebook."
  type        = string
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

variable "instance_image" {
  description = "Instance Image"
  type        = map(string)
  default = {
    project = "deeplearning-platform-release"
    family  = "tf-latest-cpu"
    name    = null
  }

  validation {
    condition     = can(coalesce(var.instance_image.project))
    error_message = "In var.instance_image, the \"project\" field must be a string set to the Cloud project ID."
  }

  validation {
    condition     = can(coalesce(var.instance_image.name)) != can(coalesce(var.instance_image.family))
    error_message = "In var.instance_image, exactly one of \"family\" or \"name\" fields must be set to desired image family or name."
  }
}

variable "gcs_bucket_path" {
  description = "Bucket name, can be provided from the google-cloud-storage module"
  type        = string
  default     = null
}

variable "mount_runner" {
  description = "mount content from the google-cloud-storage module"
  type        = map(string)
}
