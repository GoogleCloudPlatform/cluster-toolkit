/**
 * Copyright 2026 Google LLC
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

variable "region" {
  description = "The region to deploy to"
  type        = string
}

variable "labels" {
  description = "Labels to add to the GCS bucket. Key-value pairs."
  type        = map(string)
}

variable "local_mount" {
  description = "The mount point where the contents of the device may be accessed after mounting."
  type        = string
  default     = "/mnt"
}

variable "mount_options" {
  description = "Mount options to be put in fstab. Note: `implicit_dirs` makes it easier to work with objects added by other tools, but there is a performance impact. See: [more information](https://github.com/GoogleCloudPlatform/gcsfuse/blob/master/docs/semantics.md#implicit-directories)"
  type        = string
  default     = "defaults,_netdev,implicit_dirs"
}

variable "name_prefix" {
  description = "Name Prefix."
  type        = string
  default     = null
}

variable "use_deployment_name_in_bucket_name" {
  description = "If true, the deployment name will be included as part of the bucket name. This helps prevent naming clashes across multiple deployments."
  type        = bool
  default     = true
}

variable "random_suffix" {
  description = "If true, a random id will be appended to the suffix of the bucket name."
  type        = bool
  default     = false
}

variable "force_destroy" {
  description = "If true will destroy bucket with all objects stored within."
  type        = bool
  default     = false
}

variable "viewers" {
  description = "A list of additional accounts that can read packages from this bucket"
  type        = set(string)
  default     = []

  validation {
    error_message = "All bucket viewers must be in IAM style: user:user@example.com, serviceAccount:sa@example.com, or group:group@example.com."
    condition = alltrue([
      for viewer in var.viewers : length(regexall("^(user|serviceAccount|group):", viewer)) > 0
    ])
  }
}

variable "enable_hierarchical_namespace" {
  description = "If true, enables hierarchical namespace for the bucket. This option must be configured during the initial creation of the bucket."
  type        = bool
  default     = false
}

variable "uniform_bucket_level_access" {
  description = "Allow uniform control access to the bucket."
  type        = bool
  default     = true
}

variable "storage_class" {
  description = "The storage class of the GCS bucket."
  type        = string
  default     = "REGIONAL"
  validation {
    condition = contains([
      "STANDARD",
      "MULTI_REGIONAL",
      "REGIONAL",
      "NEARLINE",
      "COLDLINE",
      "ARCHIVE"
    ], var.storage_class)
    error_message = "Allowed values for GCS storage_class are 'STANDARD', 'MULTI_REGIONAL', 'REGIONAL', 'NEARLINE', 'COLDLINE', 'ARCHIVE'.\nhttps://cloud.google.com/storage/docs/storage-classes"
  }
}

variable "autoclass" {
  description = <<-EOT
  Configure bucket autoclass setup

  The autoclass config supports automatic transitions of objects in the bucket to appropriate storage classes based on each object's access pattern.

  The terminal storage class defines that objects in the bucket eventually transition to if they are not read for a certain length of time. 
  Supported values include: 'NEARLINE', 'ARCHIVE' (Default 'NEARLINE')

  See Cloud documentation for more details:

  https://cloud.google.com/storage/docs/autoclass
  EOT
  type = object({
    enabled                = optional(bool, false)
    terminal_storage_class = optional(string, null)
  })
  default = {
    enabled = false
  }
  nullable = false
  validation {
    condition     = !can(coalesce(var.autoclass.terminal_storage_class)) || var.autoclass.enabled
    error_message = "Cannot set bucket var.autoclass.terminal_storage_class unless var.autoclass.enabled is true"
  }
}

variable "public_access_prevention" {
  description = <<-EOT
  Bucket public access can be controlled by setting a value of either `inherited` or `enforced`. 
  When set to `enforced`, public access to the bucket is blocked.
  If set to `inherited`, the bucket's public access prevention depends on whether it is subject to the organization policy constraint for public access prevention.

  See Cloud documentation for more details:

  https://cloud.google.com/storage/docs/public-access-prevention
  EOT
  type        = string
  default     = null
  validation {
    condition = var.public_access_prevention == null ? true : contains([
      "inherited",
      "enforced"
    ], var.public_access_prevention)
    error_message = "Allowed values for public_access_prevention are 'inherited', 'enforced'.\n"
  }
}

variable "soft_delete_retention_duration" {
  description = <<-EOT
  If defined, this will configure soft_delete_policy with retention_duration_seconds for the bucket, value can be 0 or in between 604800(7 days) and 7776000(90 days).
  Setting a 0 duration disables soft delete, meaning any deleted objects will be permanently deleted.

  See Cloud documentation for more details:

  https://cloud.google.com/storage/docs/soft-delete
  EOT
  type        = number
  default     = null
  validation {
    condition     = var.soft_delete_retention_duration == null ? true : var.soft_delete_retention_duration == 0 || var.soft_delete_retention_duration >= 604800 && var.soft_delete_retention_duration <= 7776000
    error_message = "var.soft_delete_retention_duration value can be 0 or in between 604800(7 days) and 7776000(90 days)."
  }
}

variable "enable_versioning" {
  description = "If true, enables versioning for the bucket."
  type        = bool
  default     = false
}

variable "lifecycle_rules" {
  description = "List of config to manage data lifecycle rules for the bucket. For more details: https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket.html#nested_lifecycle_rule"
  type = list(object({
    # Object with keys:
    # - type - The type of the action of this Lifecycle Rule. Supported values: Delete and SetStorageClass.
    # - storage_class - (Required if action type is SetStorageClass) The target Storage Class of objects affected by this Lifecycle Rule.
    action = object({
      type          = string
      storage_class = optional(string)
    })

    # Object with keys:
    # - age - (Optional) Minimum age of an object in days to satisfy this condition.
    # - send_age_if_zero - (Optional) While set true, num_newer_versions value will be sent in the request even for zero value of the field.
    # - created_before - (Optional) Creation date of an object in RFC 3339 (e.g. 2017-06-13) to satisfy this condition.
    # - with_state - (Optional) Match to live and/or archived objects. Supported values include: "LIVE", "ARCHIVED", "ANY".
    # - matches_storage_class - (Optional) Comma delimited string for storage class of objects to satisfy this condition. Supported values include: MULTI_REGIONAL, REGIONAL, NEARLINE, COLDLINE, ARCHIVE, STANDARD, DURABLE_REDUCED_AVAILABILITY.
    # - matches_prefix - (Optional) One or more matching name prefixes to satisfy this condition.
    # - matches_suffix - (Optional) One or more matching name suffixes to satisfy this condition.
    # - num_newer_versions - (Optional) Relevant only for versioned objects. The number of newer versions of an object to satisfy this condition.
    # - custom_time_before - (Optional) A date in the RFC 3339 format YYYY-MM-DD. This condition is satisfied when the customTime metadata for the object is set to an earlier date than the date used in this lifecycle condition.
    # - days_since_custom_time - (Optional) The number of days from the Custom-Time metadata attribute after which this condition becomes true.
    # - days_since_noncurrent_time - (Optional) Relevant only for versioned objects. Number of days elapsed since the noncurrent timestamp of an object.
    # - noncurrent_time_before - (Optional) Relevant only for versioned objects. The date in RFC 3339 (e.g. 2017-06-13) when the object became nonconcurrent.
    condition = object({
      age                        = optional(number)
      send_age_if_zero           = optional(bool)
      created_before             = optional(string)
      with_state                 = optional(string)
      matches_storage_class      = optional(string)
      matches_prefix             = optional(string)
      matches_suffix             = optional(string)
      num_newer_versions         = optional(number)
      custom_time_before         = optional(string)
      days_since_custom_time     = optional(number)
      days_since_noncurrent_time = optional(number)
      noncurrent_time_before     = optional(string)
    })
  }))
  default = []
}

variable "retention_policy_period" {
  description = <<-EOT
  If defined, this will configure retention_policy with retention_period for the bucket, value must be in between 1 and 3155760000(100 years) seconds.

  See Cloud documentation for more details:

  https://cloud.google.com/storage/docs/bucket-lock
  EOT
  type        = number
  default     = null
  validation {
    condition     = var.retention_policy_period == null ? true : var.retention_policy_period > 0 && var.retention_policy_period <= 3155760000
    error_message = "var.soft_delete_policy_retention_duration value must be in between 1 and 3155760000(100 years) seconds."
  }
}

variable "enable_object_retention" {
  description = <<-EOT
  If true, enables retention policy at per object level for the bucket.

  See Cloud documentation for more details:

  https://cloud.google.com/storage/docs/object-lock
  EOT
  type        = bool
  default     = false
}

variable "anywhere_cache" {
  description = <<-EOT
    Anywhere Cache configurations.
    When you create a cache for a bucket, the cache must be created in a zone within the location of your bucket.
    For example, if your bucket is located in the us-east1 region, you can create a cache in us-east1-b but not us-central1-c.
    If your bucket is located in the ASIA dual-region, you can create a cache in any zones that make up the asia-east1 and asia-southeast1 regions.
    This validation only works for single regions.
    EOT
  type = object({
    zones            = list(string)
    ttl              = optional(string, "86400s")
    admission_policy = optional(string, "admit-on-first-miss")
  })
  default = null

  validation {
    condition     = var.anywhere_cache == null ? true : contains(["admit-on-first-miss", "admit-on-second-miss"], var.anywhere_cache.admission_policy)
    error_message = "Allowed policies are 'admit-on-first-miss' or 'admit-on-second-miss'."
  }

  validation {
    condition = var.anywhere_cache == null ? true : (
      can(regex("^([0-9]+)s$", var.anywhere_cache.ttl)) ? (
        tonumber(regex("^([0-9]+)s$", var.anywhere_cache.ttl)[0]) >= 86400 &&
        tonumber(regex("^([0-9]+)s$", var.anywhere_cache.ttl)[0]) <= 604800
      ) : false
    )
    error_message = "TTL must be between 1 day (86400s) and 7 days (604800s) and in the format 'Xs'."
  }

  validation {
    condition     = var.anywhere_cache == null ? true : length(var.anywhere_cache.zones) == length(distinct(var.anywhere_cache.zones))
    error_message = "Each Anywhere Cache configuration must specify a unique zone."
  }
}

variable "anywhere_cache_create_timeout" {
  description = <<-EOT
    Timeout for Anywhere Cache creation operations. Can be set to a duration like '1h' or '30m'.
    The maximum documented creation time is 48 hours. Please refer to the official documentation for more details on timeouts:
    https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_anywhere_cache#timeouts
  EOT
  type        = string
  default     = "240m" # 4 hours

  validation {
    condition     = can(regex("^[0-9]+(s|m|h)$", var.anywhere_cache_create_timeout))
    error_message = "The 'anywhere_cache_create_timeout' must be a duration string (e.g., '30s', '5m', '1h')."
  }
}
