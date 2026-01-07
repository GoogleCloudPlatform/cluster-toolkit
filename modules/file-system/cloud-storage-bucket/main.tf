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

locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "cloud-storage-bucket", ghpc_role = "file-system" })
}

locals {
  prefix         = var.name_prefix != null ? var.name_prefix : ""
  deployment     = var.use_deployment_name_in_bucket_name ? var.deployment_name : ""
  suffix         = var.random_suffix ? random_id.resource_name_suffix.hex : ""
  first_dash     = (local.prefix != "" && (local.deployment != "" || local.suffix != "")) ? "-" : ""
  second_dash    = local.deployment != "" && local.suffix != "" ? "-" : ""
  composite_name = "${local.prefix}${local.first_dash}${local.deployment}${local.second_dash}${local.suffix}"
  name           = local.composite_name == "" ? "no-bucket-name-provided" : local.composite_name
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

resource "google_storage_bucket" "bucket" {
  provider                    = google-beta
  project                     = var.project_id
  name                        = local.name
  uniform_bucket_level_access = var.uniform_bucket_level_access
  location                    = var.region
  storage_class               = var.storage_class
  labels                      = local.labels
  force_destroy               = var.force_destroy
  public_access_prevention    = var.public_access_prevention
  enable_object_retention     = var.enable_object_retention
  hierarchical_namespace {
    enabled = var.enable_hierarchical_namespace
  }

  dynamic "autoclass" {
    for_each = var.autoclass.enabled ? [1] : []
    content {
      enabled                = var.autoclass.enabled
      terminal_storage_class = var.autoclass.terminal_storage_class
    }
  }

  dynamic "soft_delete_policy" {
    for_each = var.soft_delete_retention_duration == null ? [] : [1]
    content {
      retention_duration_seconds = var.soft_delete_retention_duration
    }
  }

  dynamic "retention_policy" {
    for_each = var.retention_policy_period == null ? [] : [1]
    content {
      retention_period = var.retention_policy_period
    }
  }

  dynamic "versioning" {
    for_each = var.enable_versioning ? [1] : []
    content {
      enabled = var.enable_versioning
    }
  }

  dynamic "lifecycle_rule" {
    for_each = var.lifecycle_rules
    content {
      action {
        type          = lifecycle_rule.value.action.type
        storage_class = lookup(lifecycle_rule.value.action, "storage_class", null)
      }
      condition {
        age                        = lookup(lifecycle_rule.value.condition, "age", null)
        send_age_if_zero           = lookup(lifecycle_rule.value.condition, "send_age_if_zero", null)
        created_before             = lookup(lifecycle_rule.value.condition, "created_before", null)
        with_state                 = lookup(lifecycle_rule.value.condition, "with_state", contains(keys(lifecycle_rule.value.condition), "is_live") ? (lifecycle_rule.value.condition["is_live"] ? "LIVE" : null) : null)
        matches_storage_class      = lifecycle_rule.value.condition["matches_storage_class"] != null ? split(",", lifecycle_rule.value.condition["matches_storage_class"]) : null
        matches_prefix             = lifecycle_rule.value.condition["matches_prefix"] != null ? split(",", lifecycle_rule.value.condition["matches_prefix"]) : null
        matches_suffix             = lifecycle_rule.value.condition["matches_suffix"] != null ? split(",", lifecycle_rule.value.condition["matches_suffix"]) : null
        num_newer_versions         = lookup(lifecycle_rule.value.condition, "num_newer_versions", null)
        custom_time_before         = lookup(lifecycle_rule.value.condition, "custom_time_before", null)
        days_since_custom_time     = lookup(lifecycle_rule.value.condition, "days_since_custom_time", null)
        days_since_noncurrent_time = lookup(lifecycle_rule.value.condition, "days_since_noncurrent_time", null)
        noncurrent_time_before     = lookup(lifecycle_rule.value.condition, "noncurrent_time_before", null)
      }
    }
  }

  lifecycle {
    precondition {
      condition     = !var.autoclass.enabled || !var.enable_hierarchical_namespace
      error_message = "Hierarchical namespace is not compatible with Autoclass enabled."
    }

    precondition {
      condition     = !var.enable_hierarchical_namespace || var.uniform_bucket_level_access
      error_message = "Hierarchical namespace is not compatible with Uniform bucket level access disabled."
    }

    precondition {
      condition     = !var.enable_versioning || !var.enable_hierarchical_namespace
      error_message = "Hierarchical namespace is not compatible with Object versioning enabled."
    }

    precondition {
      condition = var.anywhere_cache != null ? alltrue([
        for zone in var.anywhere_cache.zones : startswith(zone, var.region)
      ]) : true
      error_message = "The zone for the Anywhere Cache must be within the bucket's region."
    }
  }
}

resource "google_storage_bucket_iam_binding" "viewers" {
  bucket  = google_storage_bucket.bucket.name
  role    = "roles/storage.objectViewer"
  members = var.viewers
}

resource "google_storage_anywhere_cache" "cache_instances" {
  for_each = var.anywhere_cache != null ? toset(var.anywhere_cache.zones) : toset([])

  provider = google-beta

  bucket           = google_storage_bucket.bucket.name
  zone             = each.value
  ttl              = var.anywhere_cache.ttl
  admission_policy = var.anywhere_cache.admission_policy

  timeouts {
    create = var.anywhere_cache_create_timeout
  }
}
