# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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
  values = var.values_yaml

  dynamic "set" {
    for_each = var.set_values
    content {
      name  = set.value.name
      value = set.value.value
      type  = set.value.type
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
  recreate_pods              = var.recreate_pods # Note: Deprecated in Helm CLI
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
    # Only include the block if var.postrender is not null
    for_each = var.postrender == null ? [] : [var.postrender]
    content {
      binary_path = postrender.value.binary_path
    }
  }

}
