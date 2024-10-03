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
  description = "Project in which Google Cloud Storage bucket will be created"
  type        = string
}

variable "deployment_name" {
  description = "Cluster Toolkit deployment name. Cloud resource names will include this value."
  type        = string
}

variable "region" {
  description = "Default region for creating resources"
  type        = string
}

variable "devel_rpm" {
  description = "Absolute path to PBS Pro Development RPM file"
  type        = string
  validation {
    condition     = can(regex("^/", var.devel_rpm))
    error_message = "Path to RPM must be an absolute path beginning with \"/\"."
  }
}

variable "server_rpm" {
  description = "Absolute path to PBS Pro Server Host RPM file"
  type        = string
  validation {
    condition     = can(regex("^/", var.server_rpm))
    error_message = "Path to RPM must be an absolute path beginning with \"/\"."
  }
}

variable "client_rpm" {
  description = "Absolute path to PBS Pro Client Host RPM file"
  type        = string
  validation {
    condition     = can(regex("^/", var.client_rpm))
    error_message = "Path to RPM must be an absolute path beginning with \"/\"."
  }
}

variable "execution_rpm" {
  description = "Absolute path to PBS Pro Execution Host RPM file"
  type        = string
  validation {
    condition     = can(regex("^/", var.execution_rpm))
    error_message = "Path to RPM must be an absolute path beginning with \"/\"."
  }
}

variable "license_file" {
  description = "Path to PBS Pro license file"
  type        = string
  default     = null
}

variable "location" {
  description = "Google Cloud Storage bucket location (defaults to var.region if not set)"
  type        = string
  default     = null
}

variable "storage_class" {
  description = "Google Cloud Storage class"
  type        = string
  default     = "STANDARD"
}

variable "bucket_lifecycle_rules" {
  description = "Additional lifecycle_rules for specific buckets. Map of lowercase unprefixed name => list of lifecycle rules to configure."
  type = list(object({
    # Object with keys:
    # - type - The type of the action of this Lifecycle Rule. Supported values: Delete and SetStorageClass.
    # - storage_class - (Required if action type is SetStorageClass) The target Storage Class of objects affected by this Lifecycle Rule.
    action = map(string)

    # Object with keys:
    # - age - (Optional) Minimum age of an object in days to satisfy this condition.
    # - created_before - (Optional) Creation date of an object in RFC 3339 (e.g. 2017-06-13) to satisfy this condition.
    # - with_state - (Optional) Match to live and/or archived objects. Supported values include: "LIVE", "ARCHIVED", "ANY".
    # - matches_storage_class - (Optional) Comma delimited string for storage class of objects to satisfy this condition. Supported values include: MULTI_REGIONAL, REGIONAL, NEARLINE, COLDLINE, STANDARD, DURABLE_REDUCED_AVAILABILITY.
    # - num_newer_versions - (Optional) Relevant only for versioned objects. The number of newer versions of an object to satisfy this condition.
    # - custom_time_before - (Optional) A date in the RFC 3339 format YYYY-MM-DD. This condition is satisfied when the customTime metadata for the object is set to an earlier date than the date used in this lifecycle condition.
    # - days_since_custom_time - (Optional) The number of days from the Custom-Time metadata attribute after which this condition becomes true.
    # - days_since_noncurrent_time - (Optional) Relevant only for versioned objects. Number of days elapsed since the noncurrent timestamp of an object.
    # - noncurrent_time_before - (Optional) Relevant only for versioned objects. The date in RFC 3339 (e.g. 2017-06-13) when the object became nonconcurrent.
    condition = map(string)
  }))
  # by default, retain at least 2 versions of object while and any other
  # versions younger than 14 days
  default = [{
    condition = {
      age                = 14
      num_newer_versions = 2
    }
    action = {
      type = "Delete"
    }
  }]
}

# consider value such as { "is_locked" = false, "retention_period" = 31536000 }
variable "retention_policy" {
  description = "Google Cloud Storage retention policy (to prevent accidental deletion)"
  type        = any
  default     = {}
}

variable "versioning" {
  description = "Enable versioning of Google Cloud Storage objects (cannot be enabled with a retention policy)"
  type        = bool
  default     = false
}

variable "force_destroy" {
  description = "Set to true if object versioning is enabled and you are certain that you want to destroy the bucket."
  type        = bool
  default     = false
}

variable "bucket_viewers" {
  description = "A list of additional accounts that can read packages from this bucket"
  type        = set(string)
  default     = []

  validation {
    error_message = "All bucket viewers must be in IAM style: user:user@example.com, serviceAccount:sa@example.com, or group:group@example.com."
    condition = alltrue([
      for viewer in var.bucket_viewers : length(regexall("^(user|serviceAccount|group):", viewer)) > 0
    ])
  }
}

variable "labels" {
  description = "Labels to add to the created bucket. Key-value pairs."
  type        = map(string)
}
