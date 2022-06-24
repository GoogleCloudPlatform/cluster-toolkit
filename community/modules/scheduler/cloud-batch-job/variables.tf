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

variable "job_id" {
  description = "An id for the batch job. Used for output instructions and file naming."
  type        = string
  default     = "my_job"
}

variable "gcloud_version" {
  description = "The version of the gcloud cli being used. Used for output instructions."
  type        = string
  default     = "alpha"
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
