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

# Terraform state bucket for resources provisioned by the Open Front End.
#
# A SEPARATE root from ../, applied before it and never referenced by it.
# teardown.sh removes it only with --delete-state-bucket, so tearing the front
# end down cannot destroy the state of the resources it provisioned. A resource
# whose state is gone cannot be destroyed by Terraform: it keeps running and
# keeps billing.
#
# This is NOT the control bucket (module "control_bucket" in ../main.tf), which
# stages bootstrap files and startup scripts and is force-destroy and
# unversioned so that teardown can remove it.

locals {
  default_labels = {
    ghpcfe_id = var.deployment_name,
  }
  labels = merge(var.extra_labels, local.default_labels)

  # The name depends only on project and deployment name, so deploy.sh can
  # compute it without this root's local state and adopt an existing bucket
  # instead of creating a second one. Region is excluded: a bucket cannot
  # change location. The hash of project and full deployment name keeps names
  # distinct across projects and between names that clean to the same text.
  #
  # Bucket names allow 3-63 characters of a-z, 0-9, '-' and '_', starting and
  # ending with a letter or digit ('.' needs domain verification). The
  # deployment name is lowercased, other characters become '-', it is cut to
  # fit, and leading/trailing '-' and '_' are dropped ("ofe" if nothing is
  # left). A name that is already valid and at most 46 characters is used
  # unchanged; keep it that way, or existing deployments get a new bucket.
  bucket_suffix      = "-tfstate-${substr(sha256("${var.project_id}/${var.deployment_name}"), 0, 8)}"
  bucket_name_clean  = replace(substr(replace(lower(var.deployment_name), "/[^a-z0-9_-]/", "-"), 0, 63 - length(local.bucket_suffix)), "/^[-_]+|[-_]+$/", "")
  bucket_name_prefix = local.bucket_name_clean == "" ? "ofe" : local.bucket_name_clean
  bucket_name        = "${local.bucket_name_prefix}${local.bucket_suffix}"

  writers = distinct(concat(var.state_bucket_writers, var.default_writers))
}

resource "google_storage_bucket" "tfstate" {
  name     = local.bucket_name
  project  = var.project_id
  location = var.region

  storage_class = "STANDARD"

  # The reason for a dedicated bucket: a state file truncated by a crash
  # mid-write can be rolled back to a prior generation.
  versioning {
    enabled = true
  }

  # A routine teardown must never be able to empty this bucket.
  force_destroy = false

  # State contains full resource detail and may contain generated secrets.
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"

  # Bounds versioned storage. Retention is deliberately generous, to allow
  # recovery from a mistake noticed late.
  lifecycle_rule {
    condition {
      days_since_noncurrent_time = var.noncurrent_version_retention_days
      with_state                 = "ARCHIVED"
    }
    action {
      type = "Delete"
    }
  }

  labels = local.labels

  # Removing this bucket must be deliberate, never a side effect.
  # teardown.sh --delete-state-bucket lifts this and force_destroy in a
  # temporary copy, once every state in the bucket is empty; it edits these
  # exact lines with sed, so keep their form.
  lifecycle {
    prevent_destroy = true
  }
}

# Grants access to the identity that runs Terraform: NOT the front end's own
# service account, but the one whose key is registered in the portal as a
# Credential (passed per resource as GOOGLE_APPLICATION_CREDENTIALS). It may
# belong to another project (docs/AdvancedAdmin.md), so it must be listed
# explicitly: deploy.sh supplies the provisioning service account it creates
# (default_writers); administrators add others to state_bucket_writers in
# admin.auto.tfvars.
#
# Authoritative, so the listed members are exactly the principals holding the
# role. objectAdmin on this bucket alone covers Terraform's init, plan, apply,
# destroy and state locking; no project-level role is needed.
resource "google_storage_bucket_iam_binding" "state_writers" {
  count = length(local.writers) > 0 ? 1 : 0

  bucket  = google_storage_bucket.tfstate.name
  role    = "roles/storage.objectAdmin"
  members = local.writers
}
