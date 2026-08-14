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
  is_valid      = var.cluster_id != null && var.cluster_id != ""
  cluster_parts = local.is_valid ? split("/", var.cluster_id) : []
  project       = length(local.cluster_parts) > 1 ? local.cluster_parts[1] : ""
  location      = length(local.cluster_parts) > 3 ? local.cluster_parts[3] : ""
  cluster_name  = length(local.cluster_parts) > 5 ? local.cluster_parts[5] : ""

  # Hybrid Logic: Use provided variables or fallback to data source
  endpoint = var.cluster_endpoint != null ? var.cluster_endpoint : join("", data.google_container_cluster.target[*].endpoint)
  token    = var.access_token != null ? var.access_token : join("", data.google_client_config.default[*].access_token)

  all_auth = flatten(data.google_container_cluster.target[*].master_auth)
  ca_cert  = var.cluster_ca_certificate != null ? var.cluster_ca_certificate : (length(local.all_auth) > 0 ? local.all_auth[0].cluster_ca_certificate : "")
}

data "google_client_config" "default" {
  # Skip if token is provided
  count = (local.is_valid && var.access_token == null) ? 1 : 0
}

data "google_container_cluster" "target" {
  # Skip if endpoint is provided
  count    = (local.is_valid && var.cluster_endpoint == null) ? 1 : 0
  name     = local.cluster_name
  location = local.location
  project  = local.project
}

provider "helm" {
  kubernetes {
    host                   = local.is_valid ? "https://${local.endpoint}" : null
    token                  = local.is_valid ? local.token : null
    cluster_ca_certificate = local.is_valid ? base64decode(local.ca_cert) : null
  }
}

resource "helm_release" "apply_chart" {
  # Required Identification
  name  = var.release_name
  chart = var.chart_name
  # Chart Source & Version
  repository = var.chart_repository
  version    = var.chart_version
  devel      = var.devel
  # Target Namespace
  namespace        = var.namespace
  create_namespace = var.create_namespace
  # Values Configuration
  values = [for v in var.values_yaml : fileexists(v) ? file(v) : v]
  dynamic "set" {
    for_each = var.set_values
    content {
      name  = set.value.name
      value = set.value.value
      type  = set.value.type
    }
  }
  # Implicit dependency anchor
  dynamic "set" {
    for_each = length(var.dependencies) > 0 ? [1] : []
    content {
      name  = "tf_dependency_anchor"
      value = join(",", var.dependencies)
    }
  }
  # Installation/Upgrade Behavior
  description                = var.description
  atomic                     = var.atomic
  cleanup_on_fail            = var.cleanup_on_fail
  dependency_update          = var.dependency_update
  disable_crd_hooks          = var.disable_crd_hooks
  disable_openapi_validation = var.disable_openapi_validation
  disable_webhooks           = var.disable_webhooks
  force_update               = var.force_update
  lint                       = var.lint
  max_history                = var.max_history
  recreate_pods              = var.recreate_pods
  render_subchart_notes      = var.render_subchart_notes
  reset_values               = var.reset_values
  reuse_values               = var.reuse_values
  skip_crds                  = var.skip_crds
  timeout                    = var.timeout
  wait                       = var.wait
  wait_for_jobs              = var.wait_for_jobs
  # Verification & Credentials
  keyring          = var.keyring
  pass_credentials = var.pass_credentials
  verify           = var.verify
  # Post Rendering
  dynamic "postrender" {
    for_each = var.postrender == null ? [] : [var.postrender]
    content {
      binary_path = postrender.value.binary_path
    }
  }
}
