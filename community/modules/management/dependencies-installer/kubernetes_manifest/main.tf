# Copyright 2025 "Google LLC"
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


# Apply all manifest files dynamically
resource "kubernetes_manifest" "apply_manifests" {
  # Conditional logic to determine manifest content from 'content' or 'source'
  manifest = var.source_paths != null ? yamldecode(templatefile(var.source_paths, var.template_vars)) : yamldecode(var.content)

  timeouts {
    create = var.resource_timeouts.create
    update = var.resource_timeouts.update
    delete = var.resource_timeouts.delete
  }

  dynamic "wait" {
    for_each = var.wait_for_rollout ? [1] : []
    content {

      rollout = var.wait_for_rollout
      fields  = var.wait_for_fields
    }
  }

  # Configure the 'field_manager' block dynamically
  dynamic "field_manager" {
    for_each = var.field_manager != null ? [var.field_manager] : []
    content {
      name            = field_manager.value.name
      force_conflicts = field_manager.value.force_conflicts
    }
  }

}
