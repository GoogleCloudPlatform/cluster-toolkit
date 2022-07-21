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

variable "deployment_name" {
  description = "Name of the deployment, also used for the job_id"
  type        = string
}

variable "project_id" {
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "region" {
  description = "The region in which to create the login node"
  type        = string
}

variable "labels" {
  description = "Labels to add to the login node. List key, value pairs"
  type        = any
}

variable "instance_template" {
  description = "Login VM instance template self-link. Typically supplied by a cloud-batch-job module."
  type        = string
}

variable "job_template_contents" {
  description = "The contents of the Google Cloud Batch job template. Typically supplied by a cloud-batch-job module."
  type        = string
}

variable "job_filename" {
  description = "The filename of the generated job template file. Typically supplied by a cloud-batch-job module."
  type        = string
}

variable "job_id" {
  description = "The ID for the Google Cloud Batch job. Typically supplied by a cloud-batch-job module for use in the output instructions."
  type        = string
}

variable "gcloud_version" {
  description = "The version of the gcloud cli being used. Used for output instructions. Valid inputs are `\"alpha\"`, `\"beta\"` and \"\" (empty string for default version). Typically supplied by a cloud-batch-job module."
  type        = string
  default     = "alpha"

  validation {
    condition     = contains(["alpha", "beta", ""], var.gcloud_version)
    error_message = "Allowed values for gcloud_version are 'alpha', 'beta', or '' (empty string)."
  }
}

variable "batch_job_directory" {
  description = "The path of the directory on the login node in which to place the Google Cloud Batch job template"
  type        = string
  default     = "/home/batch-jobs"
}

variable "enable_oslogin" {
  description = "Enable or Disable OS Login with \"ENABLE\" or \"DISABLE\". Set to \"INHERIT\" to inherit project OS Login setting."
  type        = string
  default     = "ENABLE"
  validation {
    condition     = var.enable_oslogin == null ? false : contains(["ENABLE", "DISABLE", "INHERIT"], var.enable_oslogin)
    error_message = "Allowed string values for var.enable_oslogin are \"ENABLE\", \"DISABLE\", or \"INHERIT\"."
  }
}
