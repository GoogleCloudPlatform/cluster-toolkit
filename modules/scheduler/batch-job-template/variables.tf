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
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "region" {
  description = "The region in which to run the Google Cloud Batch job"
  type        = string
}

variable "deployment_name" {
  description = "Name of the deployment, used for the job_id"
  type        = string
}

variable "labels" {
  description = "Labels to add to the Google Cloud Batch compute nodes. Key-value pairs. Ignored if `instance_template` is provided."
  type        = map(string)
}

variable "job_id" {
  description = "An id for the Google Cloud Batch job. Used for output instructions and file naming. Defaults to deployment name."
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

variable "task_count" {
  description = "Number of parallel tasks"
  type        = number
  default     = 1
}

variable "task_count_per_node" {
  description = "Max number of tasks that can be run on a VM at the same time. If not specified, Batch will decide a value."
  type        = number
  default     = null
}

variable "mpi_mode" {
  description = "Sets up barriers before and after runnable. In addition, sets `permissiveSsh=true`, `requireHostsFile=true`, and `taskCountPerNode=1`. `taskCountPerNode` can be overridden by `task_count_per_node`."
  type        = bool
  default     = false
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
  description = "A string to be executed as the main workload of the Google Cloud Batch job. This will be used to populate the generated template."
  type        = string
  default     = "## Add your workload here"
}

variable "instance_template" {
  description = "Compute VM instance template self-link to be used for Google Cloud Batch compute node. If provided, a number of other variables will be ignored as noted by `Ignored if instance_template is provided` in descriptions."
  type        = string
  default     = null
}

variable "subnetwork" {
  description = "The subnetwork that the Batch job should run on. Defaults to 'default' subnet. Ignored if `instance_template` is provided."
  type        = any
  default     = null
}

variable "enable_public_ips" {
  description = "If set to true, instances will have public IPs"
  type        = bool
  default     = true
}

variable "service_account" {
  description = "Service account to attach to the Google Cloud Batch compute node. Ignored if `instance_template` is provided."
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
  description = "Machine type to use for Google Cloud Batch compute nodes. Ignored if `instance_template` is provided."
  type        = string
  default     = "n2-standard-4"
}

variable "startup_script" {
  description = "Startup script run before Google Cloud Batch job starts. Ignored if `instance_template` is provided."
  type        = string
  default     = null
}

variable "network_storage" {
  description = "An array of network attached storage mounts to be configured. Ignored if `instance_template` is provided."
  type = list(object({
    server_ip             = string
    remote_mount          = string
    local_mount           = string
    fs_type               = string
    mount_options         = string
    client_install_runner = map(string)
    mount_runner          = map(string)
  }))
  default = []
}

variable "native_batch_mounting" {
  description = "Batch can mount some fs_type nativly using the 'volumes' block in the job file. If set to false, all mounting will happen through HPC Toolkit starup scripts."
  type        = bool
  default     = true
}

# Deprecated, replaced by instance_image
# tflint-ignore: terraform_unused_declarations
variable "image" {
  description = "DEPRECATED: Google Cloud Batch compute node image. Ignored if `instance_template` is provided."
  type        = any
  default     = null

  validation {
    condition     = var.image == null
    error_message = "The 'var.image' setting is deprecated, please use 'var.instance_image' with the fields 'project' and 'family' or 'name'."
  }
}

variable "instance_image" {
  description = <<-EOD
    Google Cloud Batch compute node image. Ignored if `instance_template` is provided.

    Expected Fields:
    name: The name of the image. Mutually exclusive with family.
    family: The image family to use. Mutually exclusive with name.
    project: The project where the image is hosted.
    EOD
  type        = map(string)
  default = {
    project = "batch-custom-image"
    family  = "batch-hpc-centos-7-official"
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

variable "on_host_maintenance" {
  description = "Describes maintenance behavior for the instance. If left blank this will default to `MIGRATE` except the use of GPUs requires it to be `TERMINATE`"
  type        = string
  default     = null
  validation {
    condition     = var.on_host_maintenance == null ? true : contains(["MIGRATE", "TERMINATE"], var.on_host_maintenance)
    error_message = "When set, the on_host_maintenance must be set to MIGRATE or TERMINATE."
  }
}
