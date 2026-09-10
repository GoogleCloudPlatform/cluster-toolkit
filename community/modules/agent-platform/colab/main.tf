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

data "google_client_openid_userinfo" "default" {}

resource "random_id" "suffix" {
  byte_length = 4
}

locals {
  # Sanitize deployment name to comply with GCP naming conventions (lowercase, hyphens only, no underscores)
  clean_deploy_name = lower(replace(var.deployment_name, "_", "-"))

  # Ensure the generated IDs do not exceed the 63-character limit for Vertex AI resources
  template_id  = "${substr(local.clean_deploy_name, 0, 49)}-tmpl-${random_id.suffix.hex}"
  runtime_id   = "${substr(local.clean_deploy_name, 0, 51)}-rt-${random_id.suffix.hex}"
  runtime_user = var.runtime_user != null && var.runtime_user != "" ? var.runtime_user : data.google_client_openid_userinfo.default.email

  has_mount_bucket = var.mount_gcs_bucket != null && var.mount_gcs_bucket != ""

  effective_post_startup_script_url = local.has_mount_bucket ? "gs://${var.mount_gcs_bucket}/scripts/mount_gcs_${var.deployment_name}.sh" : null
}

resource "google_storage_bucket_object" "mount_script" {
  count  = local.has_mount_bucket ? 1 : 0
  name   = "scripts/mount_gcs_${var.deployment_name}.sh"
  bucket = var.mount_gcs_bucket

  content = templatefile("${path.module}/templates/mount_gcs.sh.tfpl", {
    mount_gcs_bucket = var.mount_gcs_bucket
    mount_path       = var.mount_path
  })
}

resource "google_colab_runtime_template" "template" {
  name         = local.template_id
  display_name = local.template_id
  location     = var.region
  project      = var.project_id

  machine_spec {
    machine_type      = var.colab_machine_type
    accelerator_type  = var.accelerator_type
    accelerator_count = var.accelerator_count
  }

  data_persistent_disk_spec {
    disk_type    = var.disk_type
    disk_size_gb = var.disk_size_gb
  }

  network_spec {
    enable_internet_access = var.enable_internet_access
    network                = var.network
    subnetwork             = var.subnetwork
  }

  software_config {
    dynamic "env" {
      for_each = var.environment_variables
      content {
        name  = env.key
        value = env.value
      }
    }

    dynamic "post_startup_script_config" {
      for_each = local.effective_post_startup_script_url != null && local.effective_post_startup_script_url != "" ? [1] : []
      content {
        post_startup_script_url      = local.effective_post_startup_script_url
        post_startup_script_behavior = var.post_startup_script_behavior
      }
    }
  }

  dynamic "idle_shutdown_config" {
    for_each = var.idle_timeout != null && var.idle_timeout != "" ? [1] : []
    content {
      idle_timeout = var.idle_timeout
    }
  }

  lifecycle {
    precondition {
      condition     = (var.accelerator_type == null && var.accelerator_count == null) || (var.accelerator_type != null && var.accelerator_count != null)
      error_message = "Both accelerator_type and accelerator_count must be specified together, or both must be null."
    }
  }

  depends_on = [
    google_storage_bucket_object.mount_script,
  ]
}

resource "google_colab_runtime" "runtime" {
  name         = local.runtime_id
  display_name = local.runtime_id
  location     = var.region
  project      = var.project_id
  runtime_user = local.runtime_user

  notebook_runtime_template_ref {
    notebook_runtime_template = google_colab_runtime_template.template.id
  }
}
