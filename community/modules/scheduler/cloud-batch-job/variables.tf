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

variable "region" {
  description = "The region in which to run the Cloud Batch job"
  type        = string
}

variable "deployment_name" {
  description = "Name of the deployment, used for the job_id"
  type        = string
}

variable "labels" {
  description = "Labels to add to the Batch compute nodes. List key, value pairs. Ignored if `instance_template` is provided."
  type        = any
}

variable "job_id" {
  description = "An id for the batch job. Used for output instructions and file naming. Defaults to deployment name."
  type        = string
  default     = null
}

variable "job_filename" {
  description = "The filename of the generated job template file. Will default to `cloud-batch-<job_id>.json` if not specified"
  type        = string
  default     = null
}

variable "gcloud_version" {
  description = "The version of the gcloud cli being used. Used for output instructions. Valid inputs are `\"alpha\"`, `\"beta\"` and \"\" (empty string for default version)"
  type        = string
  default     = "alpha"

  validation {
    condition     = contains(["alpha", "beta", ""], var.gcloud_version)
    error_message = "Allowed values for gcloud_version are 'alpha', 'beta', or '' (empty string)."
  }
}

variable "log_policy" {
  description = <<-EOT
  Create a block to define log policy.
  When set to `CLOUD_LOGGING`, logs will be sent to Cloud Logging.
  When set to `PATH`, path must be added to generated template.
  When set to `DESTINATION_UNSPECIFIED`, logs will not be preserved.
  EOT
  type        = string
  default     = "CLOUD_LOGGING"

  validation {
    condition     = contains(["CLOUD_LOGGING", "PATH", "DESTINATION_UNSPECIFIED"], var.log_policy)
    error_message = "Allowed values for log_policy are 'CLOUD_LOGGING', 'PATH', or  'DESTINATION_UNSPECIFIED'."
  }
}

variable "runnable" {
  description = "A string to be executed as the main workload of the Batch job. This will be used to populate the generated template."
  type        = string
  default     = "## Add your workload here"
}

variable "instance_template" {
  description = "Compute VM instance template self-link to be used for Batch compute node. If provided, a number of other variables will be ignored as noted by `Ignored if instance_template is provided` in descriptions."
  type        = string
  default     = null
}

variable "network_self_link" {
  description = "The self link of the network to attach the Batch compute node. Ignored if `instance_template` is provided."
  type        = string
  default     = "default"
}

variable "subnetwork_self_link" {
  description = "The self link of the subnetwork to attach the Batch compute node. Ignored if `instance_template` is provided."
  type        = string
  default     = null
}

variable "service_account" {
  description = "Service account to attach to the Batch compute node. Ignored if `instance_template` is provided."
  type = object({
    email  = string,
    scopes = set(string)
  })
  default = {
    email = null
    scopes = [
      "https://www.googleapis.com/auth/devstorage.read_only",
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring.write",
      "https://www.googleapis.com/auth/servicecontrol",
      "https://www.googleapis.com/auth/service.management.readonly",
      "https://www.googleapis.com/auth/trace.append"
    ]
  }
}

variable "machine_type" {
  description = "Machine type to use for Batch compute nodes. Ignored if `instance_template` is provided."
  type        = string
  default     = "n2-standard-4"
}

variable "startup_script" {
  description = "Startup script run before Batch job starts. Ignored if `instance_template` is provided."
  type        = string
  default     = null
}

variable "network_storage" {
  description = "An array of network attached storage mounts to be configured. Ignored if `instance_template` is provided."
  type = list(object({
    server_ip     = string,
    remote_mount  = string,
    local_mount   = string,
    fs_type       = string,
    mount_options = string
  }))
  default = []
}

variable "image" {
  description = "Batch compute node image. Ignored if `instance_template` is provided."
  type = object({
    family  = string,
    project = string
  })
  default = {
    family  = "hpc-centos-7"
    project = "cloud-hpc-image-public"
  }
}
