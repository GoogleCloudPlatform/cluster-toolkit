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

  # Search across common CTK relative root depths if template_path is relative
  template_candidates = var.cloud_build_template_path != null ? [
    var.cloud_build_template_path,
    "${path.root}/${var.cloud_build_template_path}",
    "${path.root}/../${var.cloud_build_template_path}",
    "${path.root}/../../${var.cloud_build_template_path}",
    "${path.root}/../../../${var.cloud_build_template_path}",
    "${path.root}/../../../../${var.cloud_build_template_path}"
  ] : []

  resolved_template_path = var.cloud_build_template_path != null ? try(
    [for p in local.template_candidates : p if fileexists(p)][0],
    var.cloud_build_template_path
  ) : null

  # In Git mode, automatically merge repo_url, repo_ref, and cloud_build_dir into template_vars
  merged_template_vars = merge(
    {
      _REPO_URL        = local.is_git_mode ? var.repo_url : ""
      _REPO_REF        = var.repo_ref != null ? var.repo_ref : "main"
      _CLOUD_BUILD_DIR = var.cloud_build_dir != null ? var.cloud_build_dir : "."
      _INFRA_REPO_NAME = lookup(var.template_vars, "_INFRA_REPO_NAME", lookup(var.template_vars, "_REPO_NAME", ""))
    },
    var.template_vars
  )

  rendered_config = var.cloud_build_content != null ? var.cloud_build_content : (
    local.resolved_template_path != null ? templatefile(local.resolved_template_path, local.merged_template_vars) : ""
  )

  has_custom_config   = local.rendered_config != ""
  substitutions_str   = length(var.substitutions) > 0 ? join(",", [for k, v in var.substitutions : "${k}=${v}"]) : ""
  service_acct_target = var.service_account != null && var.service_account != "" ? "projects/${var.project_id}/serviceAccounts/${var.service_account}" : ""

  source_dir = local.is_local_dir_mode ? (startswith(var.cloud_build_dir, "/") ? var.cloud_build_dir : "${path.root}/${var.cloud_build_dir}") : ""
  local_source_hash = local.is_local_dir_mode && local.source_dir != "" ? try(
    sha256(join("", [for f in sort(fileset(local.source_dir, "**")) : filesha256("${local.source_dir}/${f}") if !startswith(f, ".terraform") && !startswith(f, ".ghpc") && !startswith(f, ".git")])),
    ""
  ) : ""
}

data "google_client_config" "default" {}

resource "terraform_data" "build" {
  triggers_replace = [
    var.project_id,
    var.region,
    var.cloud_build_dir != null ? var.cloud_build_dir : "",
    local.is_git_mode ? var.repo_url : "",
    var.repo_ref != null ? var.repo_ref : "",
    local.has_custom_config ? sha256(local.rendered_config) : "",
    local.local_source_hash,
    local.substitutions_str,
    local.service_acct_target,
    var.gcs_staging_dir != null ? var.gcs_staging_dir : "",
    jsonencode(var.skip_if_exists),
    jsonencode(var.triggers)
  ]

  provisioner "local-exec" {
    interpreter = ["/bin/bash", "-c"]

    environment = {
      PROJECT_ID      = var.project_id
      REGION          = var.region
      CLOUD_BUILD_DIR = var.cloud_build_dir != null ? var.cloud_build_dir : ""
      REPO_URL        = local.is_git_mode ? var.repo_url : ""
      REPO_REF        = var.repo_ref != null ? var.repo_ref : ""
      CONFIG_CONTENT  = local.rendered_config
      GCS_STAGING_DIR = var.gcs_staging_dir != null ? var.gcs_staging_dir : ""
      SERVICE_ACCOUNT = local.service_acct_target
      SUBSTITUTIONS   = local.substitutions_str
      SKIP_IF_EXISTS  = join(",", var.skip_if_exists)
      ACCESS_TOKEN    = data.google_client_config.default.access_token
    }

    command = <<-EOT
      set -e

      # Authenticate gcloud using the active Terraform Google Provider access token / ADC
      if [ -n "$ACCESS_TOKEN" ]; then
        export CLOUDSDK_AUTH_ACCESS_TOKEN="$ACCESS_TOKEN"
      elif command -v gcloud >/dev/null 2>&1; then
        ADC_TOKEN=$(gcloud auth application-default print-access-token 2>/dev/null || true)
        if [ -n "$ADC_TOKEN" ]; then
          export CLOUDSDK_AUTH_ACCESS_TOKEN="$ADC_TOKEN"
        fi
      fi

      # Check if images already exist in Artifact Registry
      if [ -n "$SKIP_IF_EXISTS" ]; then
        ALL_EXIST=true
        CHECKED_COUNT=0
        IFS=',' read -ra IMGS <<< "$SKIP_IF_EXISTS"
        for IMG in "$${IMGS[@]}"; do
          IMG=$(echo "$IMG" | xargs)
          if [ -n "$IMG" ]; then
            CHECKED_COUNT=$((CHECKED_COUNT + 1))
            echo "--> [INFO] Checking if image '$IMG' already exists in Artifact Registry..."
            if ! gcloud artifacts docker images describe "$IMG" --project="$PROJECT_ID" >/dev/null 2>&1; then
              echo "--> [INFO] Image '$IMG' not found."
              ALL_EXIST=false
              break
            fi
          fi
        done
        if [ "$ALL_EXIST" = "true" ] && [ "$CHECKED_COUNT" -gt 0 ]; then
          echo "--> [INFO] All images in skip_if_exists already exist in Artifact Registry. Skipping Cloud Build."
          exit 0
        fi
        echo "--> [INFO] One or more images do not exist. Proceeding with build."
      fi

      TMP_WORKSPACE=$(mktemp -d)
      trap "rm -rf '$TMP_WORKSPACE'" EXIT

      # Canonical absolute path helper
      to_abs_path() {
        local P="$1"
        if [ -d "$P" ]; then
          (cd "$P" && pwd -P)
        elif [ -f "$P" ]; then
          echo "$(cd "$(dirname "$P")" && pwd -P)/$(basename "$P")"
        else
          echo "$P"
        fi
      }

      # Resolve local directory with upward traversal if relative
      resolve_local_dir() {
        local SRC_PATH="$1"
        case "$SRC_PATH" in
          /*)
            to_abs_path "$SRC_PATH"
            return
            ;;
        esac

        if [ "$SRC_PATH" = "." ] || [ -z "$SRC_PATH" ]; then
          local CUR_DIR="$(pwd -P)"
          while [ "$CUR_DIR" != "/" ] && [ -n "$CUR_DIR" ]; do
            if [ -d "$CUR_DIR/.git" ] || [ -d "$CUR_DIR/.ghpc" ]; then
              to_abs_path "$CUR_DIR"
              return
            fi
            CUR_DIR=$(dirname "$CUR_DIR")
          done
          to_abs_path "$(pwd -P)"
          return
        fi

        if [ -e "$SRC_PATH" ]; then
          to_abs_path "$SRC_PATH"
          return
        fi

        local CUR_DIR="$(pwd -P)"
        while [ "$CUR_DIR" != "/" ] && [ -n "$CUR_DIR" ]; do
          if [ -e "$CUR_DIR/$SRC_PATH" ]; then
            to_abs_path "$CUR_DIR/$SRC_PATH"
            return
          fi
          CUR_DIR=$(dirname "$CUR_DIR")
        done

        to_abs_path "$SRC_PATH"
      }

      # Assemble gcloud builds submit arguments
      BUILD_ARGS=()
      BUILD_ARGS+=("--project=$PROJECT_ID")
      BUILD_ARGS+=("--region=$REGION")

      if [ -n "$CONFIG_CONTENT" ]; then
        CONFIG_FILE="$TMP_WORKSPACE/cloudbuild.yaml"
        printf '%s\n' "$CONFIG_CONTENT" > "$CONFIG_FILE"
        BUILD_ARGS+=("--config=$CONFIG_FILE")
      fi

      if [ -n "$GCS_STAGING_DIR" ]; then
        BUILD_ARGS+=("--gcs-source-staging-dir=$GCS_STAGING_DIR")
      fi

      if [ -n "$SERVICE_ACCOUNT" ]; then
        BUILD_ARGS+=("--service-account=$SERVICE_ACCOUNT")
      fi

      if [ -n "$SUBSTITUTIONS" ]; then
        BUILD_ARGS+=("--substitutions=$SUBSTITUTIONS")
      fi

      if [ -n "$REPO_URL" ] || [ -z "$CLOUD_BUILD_DIR" ]; then
        if [ -n "$REPO_URL" ]; then
          echo "--> [INFO] Submitting Cloud Build job with --no-source (remote Git: $REPO_URL, ref: $REPO_REF)..."
        else
          echo "--> [INFO] Submitting Cloud Build job with --no-source (self-contained config)..."
        fi
        gcloud builds submit --no-source "$${BUILD_ARGS[@]}"
      else
        RESOLVED_DIR=$(resolve_local_dir "$CLOUD_BUILD_DIR")
        if [ ! -d "$RESOLVED_DIR" ]; then
          echo "ERROR: Local Cloud Build source directory not found: $RESOLVED_DIR (configured cloud_build_dir: $CLOUD_BUILD_DIR)" >&2
          exit 1
        fi
        echo "--> [INFO] Submitting Cloud Build job with local source: $RESOLVED_DIR (project: $PROJECT_ID, region: $REGION)..."
        gcloud builds submit "$RESOLVED_DIR" "$${BUILD_ARGS[@]}"
      fi

      echo "--> [INFO] Cloud Build completed successfully."
    EOT
  }
}
