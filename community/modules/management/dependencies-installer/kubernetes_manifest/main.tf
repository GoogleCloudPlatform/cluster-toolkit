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

locals {
  yaml_separator = "\n---"

  # --- 1. Determine the primary source of YAML content ---
  # Prioritize 'content' variable if provided
  primary_content_body = var.content != "" ? var.content : null

  # --- 2. Handle 'source_path' based on its type (File vs. Directory) ---

  # Check if source_path is a directory (indicated by trailing slash)
  is_directory            = endswith(var.source_path, "/")
  directory_absolute_path = local.is_directory ? abspath(var.source_path) : null

  # Check if source_path is a single yaml or tftpl file (only if not a directory)
  is_single_file = !local.is_directory && (
    length(regexall("\\.yaml$", lower(var.source_path))) > 0 ||
    length(regexall("\\.tftpl$", lower(var.source_path))) > 0
  )
  single_file_raw_content = local.is_single_file ? (
    length(regexall("\\.tftpl$", lower(var.source_path))) > 0 ?
    templatefile(abspath(var.source_path), var.template_vars) :
    file(abspath(var.source_path))
  ) : null

  # Docs from primary_content_body
  docs_from_primary_source = [
    for doc in split(local.yaml_separator, coalesce(local.primary_content_body, local.single_file_raw_content, "")) : trimspace(doc)
    if length(trimspace(doc)) > 0
  ]

  # Docs from .yaml files in a directory
  directory_yaml_files = local.is_directory ? fileset(local.directory_absolute_path, "*.yaml") : []
  docs_from_directory_yamls = flatten([
    for file_name in local.directory_yaml_files :
    [
      for doc in split(local.yaml_separator, file(format("%s/%s", local.directory_absolute_path, file_name))) : trimspace(doc)
      if length(trimspace(doc)) > 0
    ]
  ])

  # Docs from .tftpl files in a directory
  directory_template_files = local.is_directory ? fileset(local.directory_absolute_path, "*.tftpl") : []
  docs_from_directory_templates = flatten([
    for file_name in local.directory_template_files :
    [
      for doc in split(local.yaml_separator, templatefile(format("%s/%s", local.directory_absolute_path, file_name), var.template_vars)) : trimspace(doc)
      if length(trimspace(doc)) > 0
    ]
  ])

  all_parsed_docs = concat(
    local.docs_from_primary_source,
    local.docs_from_directory_yamls,
    local.docs_from_directory_templates
  )

  # --- 5. Create the final map for `for_each` (keys must be unique strings) ---
  docs_map = tomap({
    for index, doc in local.all_parsed_docs : index => doc
    if length(trimspace(doc)) > 0
  })
}

# Apply all manifest files dynamically
resource "kubernetes_manifest" "apply_manifests" {
  for_each = local.docs_map
  manifest = yamldecode(each.value)
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
