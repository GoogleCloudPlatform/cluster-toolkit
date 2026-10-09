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

locals {
  is_git_mode       = var.repo_url != null && var.repo_url != "" && var.repo_url != "local"
  is_local_dir_mode = !local.is_git_mode && var.cloud_build_dir != null && var.cloud_build_dir != ""

  resolved_template_path = var.cloud_build_template_path != null && var.cloud_build_template_path != "" ? (
    startswith(var.cloud_build_template_path, "/") ? var.cloud_build_template_path : (
      fileexists("${path.root}/${var.cloud_build_template_path}")
      ? "${path.root}/${var.cloud_build_template_path}"
      : abspath("${path.root}/../../${var.cloud_build_template_path}")
    )
  ) : null

  # Automatically merge repo_url, repo_ref, cloud_build_dir, and target_dir into template_vars
  merged_template_vars = merge(
    {
      _REPO_URL        = local.is_git_mode ? var.repo_url : ""
      _REPO_REF        = var.repo_ref != null ? var.repo_ref : "main"
      _CLOUD_BUILD_DIR = var.cloud_build_dir != null ? var.cloud_build_dir : "."
      _TARGET_DIR      = var.target_dir != null ? var.target_dir : ""
      _INFRA_REPO_NAME = lookup(var.template_vars, "_INFRA_REPO_NAME", lookup(var.template_vars, "_REPO_NAME", ""))
    },
    var.template_vars
  )

  rendered_config = var.cloud_build_content != null ? var.cloud_build_content : (
    local.resolved_template_path != null ? templatefile(local.resolved_template_path, local.merged_template_vars) : ""
  )

  has_custom_config   = local.rendered_config != ""
  substitutions_str   = length(var.substitutions) > 0 ? join(",", [for k, v in var.substitutions : "${k}=${v}"]) : ""
  service_acct_target = var.service_account_email != null && var.service_account_email != "" ? "projects/${var.project_id}/serviceAccounts/${var.service_account_email}" : ""

  source_dir = local.is_local_dir_mode ? (
    startswith(var.cloud_build_dir, "/") ? var.cloud_build_dir : (
      fileexists("${path.root}/${var.cloud_build_dir}") || try(length(fileset("${path.root}/${var.cloud_build_dir}", "**")) > 0, false)
      ? "${path.root}/${var.cloud_build_dir}"
      : abspath("${path.root}/../../${var.cloud_build_dir}")
    )
  ) : ""
  local_source_hash = local.is_local_dir_mode && local.source_dir != "" ? try(
    sha256(join("", [for f in sort(fileset(local.source_dir, "**")) : filesha256("${local.source_dir}/${f}") if !can(regex("(^|/)\\.(git|ghpc|terraform)(/|$)", f))])),
    ""
  ) : ""

  has_local_target = var.is_target_dir_local && var.target_dir != null && var.target_dir != ""
  resolved_target_dir = local.has_local_target ? (
    startswith(var.target_dir, "/") ? var.target_dir : (
      fileexists("${path.root}/${var.target_dir}") || try(length(fileset("${path.root}/${var.target_dir}", "**")) > 0, false)
      ? "${path.root}/${var.target_dir}"
      : abspath("${path.root}/../../${var.target_dir}")
    )
  ) : ""
  local_overlay_hash = local.resolved_target_dir != "" ? try(
    sha256(join("", [for f in sort(fileset(local.resolved_target_dir, "**")) : filesha256("${local.resolved_target_dir}/${f}") if !can(regex("(^|/)\\.(git|ghpc|terraform)(/|$)", f))])),
    ""
  ) : ""
}

ephemeral "google_client_config" "default" {}

resource "terraform_data" "build" {
  triggers_replace = [
    var.project_id,
    var.region,
    var.cloud_build_dir != null ? var.cloud_build_dir : "",
    var.target_dir != null ? var.target_dir : "",
    tostring(var.is_target_dir_local),
    local.is_git_mode ? var.repo_url : "",
    var.repo_ref != null ? var.repo_ref : "",
    local.has_custom_config ? sha256(local.rendered_config) : "",
    local.local_source_hash,
    local.local_overlay_hash,
    local.substitutions_str,
    local.service_acct_target,
    var.gcs_staging_dir != null ? var.gcs_staging_dir : "",
    jsonencode(var.skip_if_exists),
    jsonencode(var.triggers)
  ]

  provisioner "local-exec" {
    command = "${path.module}/scripts/cloud_build.sh"

    environment = {
      PROJECT_ID      = var.project_id
      REGION          = var.region
      CLOUD_BUILD_DIR = local.is_local_dir_mode ? local.source_dir : (var.cloud_build_dir != null ? var.cloud_build_dir : "")
      REPO_URL        = local.is_git_mode ? var.repo_url : ""
      REPO_REF        = var.repo_ref != null ? var.repo_ref : ""
      CONFIG_CONTENT  = local.rendered_config
      GCS_STAGING_DIR = var.gcs_staging_dir != null ? var.gcs_staging_dir : ""
      SERVICE_ACCOUNT = local.service_acct_target
      SUBSTITUTIONS   = local.substitutions_str
      SKIP_IF_EXISTS  = join(",", var.skip_if_exists)
      ACCESS_TOKEN    = ephemeral.google_client_config.default.access_token
      TARGET_DIR      = local.resolved_target_dir
    }
  }
}
